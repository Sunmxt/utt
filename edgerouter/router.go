package edgerouter

import (
	"sync"

	"github.com/crossmesh/fabric/backend"
	"github.com/crossmesh/fabric/config"
	"github.com/crossmesh/fabric/gossip"
	"github.com/crossmesh/fabric/metanet"
	"github.com/crossmesh/fabric/proto"
	"github.com/crossmesh/fabric/route"
	logging "github.com/sirupsen/logrus"
	arbit "github.com/sunmxt/arbiter"
)

const defaultGossiperTransportBufferSize = uint(1024)

// EdgeRouter builds overlay network.
type EdgeRouter struct {
	lock sync.RWMutex

	route           route.MeshDataNetworkRouter
	metaNet         *metanet.MetadataNetwork
	overlayModel    *gossip.OverlayNetworksValidatorV1
	overlayModelKey string

	// viewpoint of global overlay networks.
	networkMap map[*metanet.MetaPeer]map[gossip.NetworkID]interface{}

	vtep *virtualTunnelEndpoint

	endpointFailures sync.Map // map[backend.Endpoint]time.Time

	configID uint32
	cfg      *config.Network
	log      *logging.Entry

	arbiters struct {
		main    *arbit.Arbiter
		config  *arbit.Arbiter
		forward *arbit.Arbiter
		metanet *arbit.Arbiter
	}
}

// New creates a new EdgeRouter.
func New(arbiter *arbit.Arbiter) (a *EdgeRouter, err error) {
	defer func() {
		if err != nil {
			arbiter.Shutdown()
			arbiter.Join()
		}
	}()

	a = &EdgeRouter{
		log:        logging.WithField("module", "edge_router"),
		vtep:       newVirtualTunnelEndpoint(nil),
		networkMap: make(map[*metanet.MetaPeer]map[gossip.NetworkID]interface{}),
	}
	a.arbiters.main = arbit.NewWithParent(arbiter)
	a.arbiters.config = arbit.New()
	a.arbiters.metanet = arbit.NewWithParent(arbiter)

	if a.metaNet, err = metanet.NewMetadataNetwork(a.arbiters.metanet, a.log.WithField("module", "metanet")); err != nil {
		return nil, err
	}
	a.metaNet.RegisterMessageHandler(proto.MsgTypeRawFrame, a.receiveRemote)

	if err = a.initializeNetworkMap(); err != nil {
		return nil, err
	}

	a.waitCleanUp()
	return a, nil
}

// SeedPeer adds seed endpoint.
func (r *EdgeRouter) SeedPeer(endpoints ...backend.Endpoint) error {
	return r.metaNet.SeedEndpoints(endpoints...)
}

func (r *EdgeRouter) waitCleanUp() {
	r.arbiters.main.Go(func() {
		<-r.arbiters.main.Exit() // watch exit signal.

		r.lock.Lock()
		defer r.lock.Unlock()

		fa := r.arbiters.forward

		// terminate forwarding.
		if fa != nil {
			fa.Shutdown()
		}

		// close vtep.
		if err := r.vtep.Close(); err != nil {
			r.log.Warn("cannot close vtep: ", err)
		}

		if fa != nil {
			fa.Join()
		}
		r.log.Debug("forwarding stopped.")

		r.arbiters.metanet.Shutdown()
		r.arbiters.metanet.Join()
		r.log.Debug("metadata network stopped.")

		r.arbiters.config.Shutdown()
		r.arbiters.config.Join()
		r.log.Debug("config stopped.")

		r.log.Debug("edgerouter cleaned up.")
	})
}

// Mode returns name of edge router working mode.
// values can be: ethernet, overlay.
func (r *EdgeRouter) Mode() string {
	cfg := r.cfg
	if cfg == nil {
		return "unknown"
	}
	mode := r.cfg.Mode
	if mode == "overlay" {
		mode = "ip"
	}
	return mode
}
