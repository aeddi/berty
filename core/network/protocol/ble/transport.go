package ble

import (
	"context"
	"fmt"

	bledrv "berty.tech/core/network/protocol/ble/driver"
	blema "berty.tech/core/network/protocol/ble/multiaddr"

	"github.com/gofrs/uuid"
	tpt "github.com/libp2p/go-libp2p-core/transport"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
)

const DefaultBind = "/ble/00000000-0000-0000-0000-000000000000"

// Transport is a BLE tpt.transport.
var _ tpt.Transport = &Transport{}

// Transport represents any device by which you can connect to and accept
// connections from other peers.
type Transport struct {
	host     host.Host
	upgrader *tptu.Upgrader
}

// NewTransport creates a BLE transport object that tracks dialers and listener.
// It also starts the discovery service.
func NewTransport(h host.Host, u *tptu.Upgrader) (*Transport, error) {
	logger().Debug("NEWTRANP CALLED 424242")
	defer logger().Debug("NEWTRANP ENDED 424242")
	return &Transport{
		host:     h,
		upgrader: u,
	}, nil
}

// Dial dials the peer at the remote address.
// With BLE you can only dial a device that is already connected with the native driver.
func (t *Transport) Dial(ctx context.Context, rMa ma.Multiaddr, rPID peer.ID) (tpt.CapableConn, error) {
	logger().Debug("DIAL CALLED 424242")
	defer logger().Debug("DIAL ENDED 424242")
	// BLE transport needs to have a running listener in order to dial other peer
	// because native driver is initialized during listener creation.
	if gListener == nil {
		logger().Error("DIAL ERR NO LIST 424242")
		return nil, errors.New("transport dialing peer failed: no active listener")
	}

	rAddr, err := rMa.ValueForProtocol(blema.P_BLE)
	if err != nil {
		logger().Error("DIAL ERR WRONG MA 424242")
		return nil, errors.Wrap(err, "transport dialing peer failed: wrong multiaddr")
	}

	// Check if native driver is already connected to peer's device.
	// With BLE you can't really dial, only auto-connect with peer nearby.
	if bledrv.DialDevice(rAddr) == false {
		logger().Error("DIAL ERR CANT DIAL 424242")
		return nil, errors.New("transport dialing peer failed: peer not connected through BLE")
	}

	// Can't have two connections on the same multiaddr
	if _, ok := connMap.Load(rAddr); ok {
		logger().Error("DIAL ERR ALREADY CONN 424242")
		return nil, errors.New("transport dialing peer failed: already connected to this address")
	}

	// Returns an outbound conn.
	return newConn(ctx, t, rMa, rPID, false)
}

// CanDial returns true if this transport believes it can dial the given
// multiaddr.
func (t *Transport) CanDial(addr ma.Multiaddr) bool {
	return blema.BLE.Matches(addr)
}

// Listen listens on the given multiaddr.
// BLE can't listen on more than one listener.
func (t *Transport) Listen(lMa ma.Multiaddr) (tpt.Listener, error) {
	logger().Debug("TRANSPLISTEN CALLED 424242")
	defer logger().Debug("TRANSPLISTEN ENDED 424242")
	// If a global listener already exists, returns an error.
	if gListener != nil {
		logger().Error("TRANSPLISTEN ERR 1 LIST 424242")
		return nil, errors.New("transport listen failed: one listener maximum")
	}

	// Checks if lMa is a valid multiaddr
	_, err := lMa.ValueForProtocol(blema.P_BLE)
	if err != nil {
		logger().Error("TRANSPLISTEN ERR WRONG MA 424242")
		return nil, errors.Wrap(err, "transport listen failed: wrong multiaddr")
	}

	// Replaces default bind by a deterministic one based on local peerID.
	if lMa.String() == DefaultBind {
		lAddr := uuid.NewV5(uuid.UUID{}, t.host.ID().String()).String()
		lMa, err = ma.NewMultiaddr(fmt.Sprintf("/ble/%s", lAddr))
		if err != nil { // Should never append.
			panic(err)
		}
	}

	return newListener(lMa, t)
}

// Proxy returns true if this transport proxies.
func (t *Transport) Proxy() bool {
	return false
}

// Protocols returns the set of protocols handled by this transport.
func (t *Transport) Protocols() []int {
	return []int{blema.P_BLE}
}

func (t *Transport) String() string {
	return "BLE"
}
