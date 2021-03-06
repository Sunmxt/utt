package route

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"

	"github.com/crossmesh/fabric/common"
)

var (
	ErrInvalidPeerID = errors.New("invalid ID of peer")
	ErrInvalidPeer   = errors.New("invalid peer")
)

type p2pL3IPv4MeshPeerRef struct {
	lock sync.RWMutex

	peer  MeshNetPeer
	ipSet map[[4]byte]struct{}
}

type p2pL3IPv4CIDRRoute struct {
	cidr net.IPNet
	peer MeshNetPeer
}

// P2PL3IPv4MeshNetworkRouter implements symmetry peer-to-peer ipv4 network.
type P2PL3IPv4MeshNetworkRouter struct {
	lock       sync.RWMutex
	ip2Peer    map[[4]byte]MeshNetPeer          // (copy-on-write)
	peers      map[string]*p2pL3IPv4MeshPeerRef // (copy-on-write)
	cidrRoutes []*p2pL3IPv4CIDRRoute            // (copy-on-write)
}

//NewP2PL3IPv4MeshNetworkRouter initializes new P2PL3IPv4MeshNetworkRouter.
func NewP2PL3IPv4MeshNetworkRouter() *P2PL3IPv4MeshNetworkRouter {
	return &P2PL3IPv4MeshNetworkRouter{
		peers:   make(map[string]*p2pL3IPv4MeshPeerRef),
		ip2Peer: make(map[[4]byte]MeshNetPeer),
	}
}

// Route routes packet.
// param `packet` is ipv4 packet.
func (r *P2PL3IPv4MeshNetworkRouter) Route(packet []byte, from MeshNetPeer) (peers []MeshNetPeer) {
	// This function will be massively called. Be careful for performance penalty.

	var dst, src [4]byte
	if len(packet) < 20 {
		// packet too small.
		return
	}
	if version := uint8(packet[0]) >> 4; version != 4 {
		// not version 4.
		return
	}

	ip2Peer, peerSet, cidrRoutes := r.ip2Peer, r.peers, r.cidrRoutes // for lock-free read, must copy a reference first.

	fromRef, _ := peerSet[from.HashID()]
	if fromRef == nil {
		// drop packet from an unknown peer.
		return
	}

	copy(dst[:], packet[16:20])
	ip := net.IP(dst[0:4])
	if !ip.IsLoopback() && !ip.IsMulticast() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() {

		// lookup.
		if !ip.Equal(net.IPv4bcast) { // unicast.
			peer, hasPeer := ip2Peer[dst]
			if hasPeer && peer != nil {
				peers = []MeshNetPeer{peer}
			}
		}
		if len(peers) < 1 { // lookup static CIDR routes.
			for _, route := range cidrRoutes {
				if route.cidr.Contains(ip) {
					peers = []MeshNetPeer{route.peer}
					break
				}
			}
		}
		if len(peers) < 1 { // boardcast.
			for _, ref := range peerSet {
				if peer := ref.peer; from.IsSelf() != peer.IsSelf() {
					peers = append(peers, peer)
				}
			}
		}
	} // else {
	// drop lookback and unspecified address.
	// multicast is not supported yet.
	// }

	// learn.
	copy(src[:], packet[12:16])
	ip = net.IP(src[0:4])
	if ip.IsLoopback() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.Equal(net.IPv4bcast) {
		// not learn lookback, unspecified and multicast/boardcast address.
		return
	}

	origin, hasRoute := ip2Peer[src]
	if hasRoute && origin == from { // exists.
		return
	}

	// try to update routes.
	r.lock.Lock()

	ip2Peer, peerSet = r.ip2Peer, r.peers
	if origin, hasRoute = ip2Peer[src]; hasRoute && origin == from { // exists.
		r.lock.Unlock()
		return
	}
	if origin != nil {
		if ref, _ := peerSet[origin.HashID()]; ref != nil { // should has peer.
			ref.lock.Lock()
			delete(ref.ipSet, src)
			ref.lock.Unlock()
		}
	}
	fromRef.lock.Lock()
	fromRef.ipSet[src] = struct{}{}
	fromRef.lock.Unlock()

	// route updates.
	newRoutes := make(map[[4]byte]MeshNetPeer, len(ip2Peer))
	for dst, peer := range ip2Peer {
		newRoutes[dst] = peer
	}
	newRoutes[src] = from
	r.ip2Peer = newRoutes // replace the old.

	r.lock.Unlock()

	return
}

// PeerJoin joins new peer.
func (r *P2PL3IPv4MeshNetworkRouter) PeerJoin(peer MeshNetPeer) {
	if peer == nil {
		return
	}
	id := peer.HashID()
	if id == "" {
		return
	}

	peers := r.peers
	if ref, hasPeer := peers[id]; hasPeer && peer == ref.peer {
		return
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	peers = r.peers
	if ref, hasPeer := peers[id]; hasPeer && peer == ref.peer {
		return
	}
	newPeers := make(map[string]*p2pL3IPv4MeshPeerRef, len(peers))
	for id, peer := range peers {
		newPeers[id] = peer
	}
	newPeers[id] = &p2pL3IPv4MeshPeerRef{
		peer:  peer,
		ipSet: map[[4]byte]struct{}{},
	}
	r.peers = newPeers
}

// PeerLeave removes peer and related routes.
func (r *P2PL3IPv4MeshNetworkRouter) PeerLeave(peer MeshNetPeer) {
	if peer == nil {
		return
	}
	id := peer.HashID()
	if id == "" {
		return
	}

	peers := r.peers
	if ref, hasPeer := peers[id]; !hasPeer || peer != ref.peer {
		return
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	peers, ip2Peer := r.peers, r.ip2Peer
	ref, hasPeer := peers[id]
	if !hasPeer || peer != ref.peer {
		return
	}

	ref.lock.Lock()
	// route updates.
	newRoutes := make(map[[4]byte]MeshNetPeer, len(ip2Peer))
	for rip, peer := range newRoutes {
		if _, exist := ref.ipSet[rip]; exist {
			continue
		}
		newRoutes[rip] = peer
	}
	r.ip2Peer = newRoutes

	cidrRoutes, newCIDRRoutes := r.cidrRoutes, ([]*p2pL3IPv4CIDRRoute)(nil)
	for i := 0; i < len(cidrRoutes); i++ {
		route := cidrRoutes[i]
		if route.peer == peer {
			if newCIDRRoutes == nil {
				cap := i
				if cap < 1 {
					cap = 1
				}
				newCIDRRoutes = make([]*p2pL3IPv4CIDRRoute, 0, cap)
				newCIDRRoutes = append(newCIDRRoutes, cidrRoutes[:i]...)
			}
			continue
		}
		if newCIDRRoutes != nil {
			newCIDRRoutes = append(newCIDRRoutes, cidrRoutes[i])
		}
	}
	if newCIDRRoutes != nil {
		r.cidrRoutes = newCIDRRoutes
	}

	ref.lock.Unlock()

	// peer updates.
	newPeers := make(map[string]*p2pL3IPv4MeshPeerRef, len(peers))
	for pid, peer := range peers {
		if pid == id {
			continue
		}
		newPeers[pid] = peer
	}
	r.peers = newPeers
}

// RemoveStaticCIDRRoutes removes static CIDR prefix routes.
func (r *P2PL3IPv4MeshNetworkRouter) RemoveStaticCIDRRoutes(peer MeshNetPeer, routes ...*net.IPNet) bool {
	if len(routes) < 1 {
		return false
	}
	if peer == nil {
		return false
	}

	cidrRoutes, newCIDRRoutes := r.cidrRoutes, ([]*p2pL3IPv4CIDRRoute)(nil)
	set := common.IPNetSet(routes)
	set = set.Clone()
	set.Build()
	for i, copyPtr := 0, 0; i < len(cidrRoutes); i++ {
		route := cidrRoutes[i]
		if route.peer == peer {
			if idx := sort.Search(set.Len(), func(i int) bool {
				le, ge := common.IPNetLess(&route.cidr, set[i]), common.IPNetLess(set[i], &route.cidr)
				return le == ge || ge
			}); idx < set.Len() {
				if possibleRoute := set[idx]; bytes.Compare(possibleRoute.IP, route.cidr.IP) == 0 &&
					bytes.Compare(possibleRoute.Mask, route.cidr.Mask) == 0 {
					continue
				}
			}
		}

		if newCIDRRoutes == nil {
			if copyPtr != i {
				newCIDRRoutes = append(newCIDRRoutes, cidrRoutes[:copyPtr]...)
			}
		} else {
			newCIDRRoutes = append(newCIDRRoutes, route)
		}
		copyPtr++
	}

	if newCIDRRoutes != nil {
		r.cidrRoutes = newCIDRRoutes
		return true
	}

	return false
}

// AddStaticCIDRRoutes add static CIDR prefix routes.
func (r *P2PL3IPv4MeshNetworkRouter) AddStaticCIDRRoutes(peer MeshNetPeer, routes ...*net.IPNet) error {
	if len(routes) < 1 {
		return nil
	}
	if peer == nil {
		return nil
	}
	id := peer.HashID()
	if id == "" {
		return ErrInvalidPeerID
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	peers, cidrRoutes := r.peers, r.cidrRoutes
	ref := peers[peer.HashID()]
	if ref.peer != peer {
		return ErrInvalidPeer
	}
	set := make(common.IPNetSet, 0, len(cidrRoutes)+len(routes))
	for _, route := range cidrRoutes {
		set = append(set, &route.cidr)
	}
	for _, route := range routes {
		if route == nil {
			continue
		}
		set = append(set, route)
	}
	if overlapped, n1, n2 := common.IPNetOverlapped(set...); overlapped {
		return fmt.Errorf("route CIDR %v and route CIDR %v are overlapped in range", n1.String(), n2.String())
	}
	for _, cidr := range routes { // just append.
		cidrRoutes = append(cidrRoutes, &p2pL3IPv4CIDRRoute{
			cidr: *cidr,
			peer: peer,
		})
	}
	r.cidrRoutes = cidrRoutes

	return nil
}
