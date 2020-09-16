package gossip

import (
	"encoding/json"
	"sort"

	"github.com/crossmesh/fabric/backend"
	"github.com/crossmesh/sladder"
)

// NetworkEndpointV1 contains a peer's network endpoint.
type NetworkEndpointV1 struct {
	Type     backend.Type `json:"t"`
	Endpoint string       `json:"ep"`
	Priority uint32       `json:"pri"`
}

// NetworkEndpointsV1Version is version value of NetworkEndpointsV1.
const NetworkEndpointsV1Version = uint16(1)

// NetworkEndpointSetV1 contains slice of NetworkEndpointV1.
type NetworkEndpointSetV1 []*NetworkEndpointV1

// Len is the number of elements in the collection.
func (l NetworkEndpointSetV1) Len() int { return len(l) }

func networkEndpointV1Less(a, b *NetworkEndpointV1) bool {
	if a == nil {
		return false
	} else if a == nil {
		return true
	}
	if a.Priority < b.Priority {
		return true
	} else if a.Priority > b.Priority {
		return false
	}
	if a.Type < b.Type {
		return true
	} else if a.Type > b.Type {
		return false
	}
	if a.Endpoint < b.Endpoint {
		return true
	}
	return false
}

func networkEndpointV1Equal(a, b *NetworkEndpointV1) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (l NetworkEndpointSetV1) Less(i, j int) bool { return networkEndpointV1Less(l[i], l[j]) }

// Equal checks whether two set are equal.
func (l NetworkEndpointSetV1) Equal(s NetworkEndpointSetV1) bool {
	if len(l) != len(s) {
		return false
	}
	for idx := range l {
		if !networkEndpointV1Equal(l[idx], s[idx]) {
			return false
		}
	}
	return true
}

// Merge merges a given set into current set.
func (l *NetworkEndpointSetV1) Merge(r NetworkEndpointSetV1, copy bool) (changed bool) {
	changed = false

	*l = (*l)[:sort.Search(len(*l), func(i int) bool { return (*l)[i] == nil })]
	r = r[:sort.Search(len(r), func(i int) bool { return r[i] == nil })]

	*l = append(*l, r...) // expand.
	lh, rh := len(*l), len(r)
	widx, last := lh, (*NetworkEndpointV1)(nil)
	write := func(ele *NetworkEndpointV1) bool {
		if networkEndpointV1Equal(last, ele) {
			return false
		}
		(*l)[widx-1] = ele
		widx--
		last = ele
		return true
	}
	for lh > 0 && rh > 0 {
		le, re := (*l)[lh-1], r[rh-1]
		lle, rle := networkEndpointV1Less(le, re), networkEndpointV1Less(re, le)
		if lle == rle { // equal.
			write(le)
			lh--
			rh--
		} else if lle { // right is bigger.
			if write(re) {
				changed = true
			}
			rh--
		} else { // left is bigger.
			write(le)
			lh--
		}
	}
	for ; rh > 0; rh-- {
		if write(r[rh-1]) {
			changed = true
		}
	}
	if widx-lh > 0 { // not full. move to front.
		for widx <= len(*l) {
			(*l)[lh-1] = (*l)[widx-1]
			lh++
			widx++
		}
		*l = (*l)[:lh]
	}
	return
}

// Swap swaps the elements with indexes i and j.
func (l NetworkEndpointSetV1) Swap(i, j int) { l[j], l[i] = l[i], l[j] }

// Build fixes data order.
func (l *NetworkEndpointSetV1) Build() {
	if len(*l) > 1 {
		sort.Sort(l)
		// deduplicate.
		rear := 1
		for last, head := (*l)[0], 1; head < len(*l); head++ {
			if (*l)[head] == nil { // nil will be at the end of slice. it should be removed.
				break
			}
			if last.Type == (*l)[head].Type && last.Endpoint == (*l)[head].Endpoint {
				continue
			}
			(*l)[rear] = (*l)[head]
			rear++
		}
		(*l) = (*l)[:rear]
	}
}

// NetworkEndpointsV1 includes peer's network endpoints for metadata exchanges.
type NetworkEndpointsV1 struct {
	Version   uint16               `json:"v"`
	Endpoints NetworkEndpointSetV1 `json:"eps"`
}

// Clone makes a deep copy.
func (v1 *NetworkEndpointsV1) Clone() (new *NetworkEndpointsV1) {
	new = &NetworkEndpointsV1{}
	new.Version = v1.Version
	new.Endpoints = append(new.Endpoints, v1.Endpoints...)
	return
}

// Equal checks whether two NetworkEndpointsV1 are equal.
func (v1 *NetworkEndpointsV1) Equal(r *NetworkEndpointsV1) bool {
	return v1 == r ||
		(v1 != nil && r != nil &&
			v1.Version == r.Version && v1.Endpoints.Equal(r.Endpoints))
}

// EncodeString trys to marshal content to string.
func (v1 *NetworkEndpointsV1) EncodeString() (string, error) {
	buf, err := v1.Encode()
	if err != nil {
		return "", err
	}
	return string(buf), err
}

// Encode trys to marshal content to bytes.
func (v1 *NetworkEndpointsV1) Encode() ([]byte, error) {
	return json.Marshal(v1)
}

// Decode trys to unmarshal structure from bytes.
func (v1 *NetworkEndpointsV1) Decode(x []byte) (err error) {
	if err = json.Unmarshal(x, v1); err != nil {
		return err
	}
	v1.Endpoints.Build()
	return
}

// DecodeString trys to unmarshal structure from string.
func (v1 *NetworkEndpointsV1) DecodeString(x string) (err error) {
	if x == "" {
		x = "{\"v\": 1}" // initial.
	}
	return v1.Decode([]byte(x))
}

// DecodeStringAndValidate trys to unmarshal structure from string and do validation.
func (v1 *NetworkEndpointsV1) DecodeStringAndValidate(x string) (err error) {
	if err = v1.DecodeString(x); err != nil {
		return
	}
	return v1.Validate()
}

// Validate validates fields.
func (v1 *NetworkEndpointsV1) Validate() error {
	if actual := v1.Version; actual != NetworkEndpointsV1Version {
		return &ModelVersionUnmatchedError{Name: "NetworkEndpointsV1", Actual: actual, Expected: NetworkEndpointsV1Version}
	}
	return nil
}

// DefaultNetworkEndpointKey is default network endpoint key for gossip data model.
const DefaultNetworkEndpointKey = "metadata_endpoint"

// NetworkEndpointsValidatorV1 implements NetworkEndpointsV1 model.
type NetworkEndpointsValidatorV1 struct{}

func (v1 NetworkEndpointsValidatorV1) presync(
	local, remote *sladder.KeyValue,
	localV1, remoteV1 *NetworkEndpointsV1) (changed bool, err error) {
	if remote == nil {
		return true, nil
	}
	if err = remoteV1.DecodeStringAndValidate(remote.Value); err != nil {
		return false, err
	}
	if err = localV1.DecodeStringAndValidate(local.Value); err != nil {
		// the local is invalid. replace it directly.
		local.Value = remote.Value
		return true, nil
	}
	return false, nil
}

func (v1 NetworkEndpointsValidatorV1) syncNormal(
	local, remote *sladder.KeyValue,
	localV1, remoteV1 *NetworkEndpointsV1) (changed bool, err error) {
	if localV1.Endpoints.Equal(remoteV1.Endpoints) {
		return false, nil
	}
	local.Value = remote.Value
	return true, nil
}

// Sync merges state of NetworkEndpointsValidatorV1 to local.
func (v1 NetworkEndpointsValidatorV1) Sync(local, remote *sladder.KeyValue) (changed bool, err error) {
	localV1, remoteV1 := NetworkEndpointsV1{}, NetworkEndpointsV1{}
	if changed, err = v1.presync(local, remote, &localV1, &remoteV1); changed || err != nil {
		return
	}
	return v1.syncNormal(local, remote, &localV1, &remoteV1)
}
func (v1 NetworkEndpointsValidatorV1) mergeSync(
	local, remote *sladder.KeyValue,
	localV1, remoteV1 *NetworkEndpointsV1) (changed bool, err error) {
	if localV1.Endpoints.Merge(remoteV1.Endpoints, false) {
		var newValue string
		if newValue, err = localV1.EncodeString(); err != nil {
			return false, err
		}
		local.Value = newValue
		return true, nil
	}
	return false, nil
}

// SyncEx merges state of NetworkEndpointsV1 to local using extended properties.
func (v1 NetworkEndpointsValidatorV1) SyncEx(local, remote *sladder.KeyValue, props sladder.KVMergingProperties) (changed bool, err error) {
	if props.Concurrent() && remote == nil {
		// existance wins.
		return false, nil
	}
	localV1, remoteV1 := NetworkEndpointsV1{}, NetworkEndpointsV1{}
	if changed, err = v1.presync(local, remote, &localV1, &remoteV1); changed || err != nil {
		return
	}
	if props.Concurrent() {
		return v1.mergeSync(local, remote, &localV1, &remoteV1)
	}
	return v1.syncNormal(local, remote, &localV1, &remoteV1)
}

// Validate does validation for NetworkEndpointsV1.
func (v1 NetworkEndpointsValidatorV1) Validate(kv sladder.KeyValue) bool {
	v := NetworkEndpointsV1{}
	return v.DecodeStringAndValidate(kv.Value) == nil
}

// NetworkEndpointsV1Txn is KVTransaction of NetworkEndpointsV1
type NetworkEndpointsV1Txn struct {
	originRaw string
	origin    *NetworkEndpointsV1
	new       *NetworkEndpointsV1 // (copy-on-write)
}

// Txn starts a transaction for NetworkEndpointsV1.
func (v1 NetworkEndpointsValidatorV1) Txn(kv sladder.KeyValue) (sladder.KVTransaction, error) {
	txn := &NetworkEndpointsV1Txn{origin: &NetworkEndpointsV1{}}
	if err := txn.origin.DecodeStringAndValidate(kv.Value); err != nil {
		return nil, err
	}
	txn.new = txn.origin
	txn.originRaw = kv.Value
	return txn, nil
}

func (t *NetworkEndpointsV1Txn) copyOnWrite() {
	if t.new == t.origin {
	}
}

// Updated returns whether any change exists.
func (t *NetworkEndpointsV1Txn) Updated() bool {
	return t.new != t.origin && !t.new.Equal(t.origin)
}

// After encodes NetworkEndpointsV1 and returns new value.
func (t *NetworkEndpointsV1Txn) After() string {
	bin, err := t.new.Encode()
	if err != nil {
		panic(err) // should not happen.
	}
	return string(bin)
}

// Before returns origin raw value.
func (t *NetworkEndpointsV1Txn) Before() string { return t.originRaw }

// SetRawValue set new raw value.
func (t *NetworkEndpointsV1Txn) SetRawValue(s string) error {
	newV1 := &NetworkEndpointsV1{}
	if err := newV1.DecodeStringAndValidate(s); err != nil {
		return err
	}
	t.new = newV1
	return nil
}

// AddEndpoints appends new network endpoints.
func (t *NetworkEndpointsV1Txn) AddEndpoints(eps ...backend.Endpoint) bool {
	if len(eps) < 1 {
		return false
	}

	t.copyOnWrite()

	var newSet NetworkEndpointSetV1
	for _, ep := range eps {
		newSet = append(newSet, &NetworkEndpointV1{
			Type:     ep.Type,
			Endpoint: ep.Endpoint,
		})
	}
	newSet.Build()

	return t.new.Endpoints.Merge(newSet, true)
}

// UpdateEndpoints applies new network endpoints.
func (t *NetworkEndpointsV1Txn) UpdateEndpoints(endpoints ...NetworkEndpointV1) {
	t.copyOnWrite()
	t.new.Endpoints = nil
	for _, endpoint := range endpoints {
		t.new.Endpoints = append(t.new.Endpoints, &NetworkEndpointV1{
			Type:     endpoint.Type,
			Endpoint: endpoint.Endpoint,
			Priority: endpoint.Priority,
		})
	}
	t.new.Endpoints.Build()
}