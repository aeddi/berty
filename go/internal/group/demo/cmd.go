package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"os"
	"path"
	"sync"

	orbitdb "berty.tech/go-orbit-db"
	"berty.tech/go-orbit-db/events"
	"berty.tech/go-orbit-db/stores"
	"berty.tech/go/internal/group"
	"berty.tech/go/internal/ipfsutil"
	"berty.tech/go/internal/orbitutil"
	"berty.tech/go/internal/orbitutil/orbitutilapi"
	"github.com/libp2p/go-libp2p-core/crypto"
)

func issueNewInvitation(device crypto.PrivKey, g *group.Group) {
	newI, err := group.NewInvitation(device, g)
	if err != nil {
		panic(err)
	}

	newIB64, err := newI.Marshal()
	if err != nil {
		panic(err)
	}

	println("New invitation: ", base64.StdEncoding.EncodeToString(newIB64))

}

func listMembers(s orbitutilapi.MemberStore) {
	members, err := s.ListMembers()
	if err != nil {
		panic(err)
	}

	println(fmt.Sprintf("Printing list of %d members", len(members)))

	for _, m := range members {
		memberKeyBytes, err := m.Member.Raw()
		if err != nil {
			panic(err)
		}

		deviceKeyBytes, err := m.Device.Raw()
		if err != nil {
			panic(err)
		}

		println("  >>  ", base64.StdEncoding.EncodeToString(memberKeyBytes), " >> ", base64.StdEncoding.EncodeToString(deviceKeyBytes))
	}
}

func mainLoop(invitation *group.Invitation, create bool) {
	//zaptest.Level(zapcore.DebugLevel)
	//config := zap.NewDevelopmentConfig()
	//config.OutputPaths = []string{"stdout"}
	//logger, _ := config.Build()
	//zap.ReplaceGlobals(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := createBuildConfig()
	if err != nil {
		panic(err)
	}

	api, err := ipfsutil.NewConfigurableCoreAPI(ctx, cfg, ipfsutil.OptionMDNSDiscovery)
	if err != nil {
		panic(err)
	}

	self, err := api.Key().Self(ctx)
	if err != nil {
		panic(err)
	}

	println("My own peer ID is", self.ID().String())

	g, err := invitation.GetGroup()
	if err != nil {
		panic(err)
	}

	p := path.Join(os.TempDir(), base64.StdEncoding.EncodeToString(invitation.InvitationPrivKey))

	odb, err := orbitutil.NewBertyOrbitDB(ctx, api, &orbitdb.NewOrbitDBOptions{Directory: &p})
	if err != nil {
		panic(err)
	}

	groupStores, err := odb.InitStoresForGroup(ctx, g, &orbitdb.CreateDBOptions{
		Directory: &p,
	})
	if err != nil {
		panic(err)
	}

	member, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		panic(err)
	}

	device, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		panic(err)
	}

	counter, err := rand.Int(rand.Reader, big.NewInt(0).SetUint64(math.MaxUint64))
	if err != nil {
		panic(err)
	}

	derivationState := make([]byte, 32)
	_, err = rand.Read(derivationState)
	if err != nil {
		panic(err)
	}

	secret := group.DeviceSecret{
		DerivationState: derivationState,
		Counter:         counter.Uint64(),
	}

	_ = secret

	memberKeyBytes, err := member.GetPublic().Raw()
	if err != nil {
		panic(err)
	}

	deviceKeyBytes, err := device.GetPublic().Raw()
	if err != nil {
		panic(err)
	}

	inviterDevicePubKey, err := invitation.GetInviterDevicePublicKey()
	if err != nil {
		panic(err)
	}

	println("Own member key:", base64.StdEncoding.EncodeToString(memberKeyBytes), "device key: ", base64.StdEncoding.EncodeToString(deviceKeyBytes))

	s := groupStores.GetMemberStore()

	if !create {
		println("Waiting store replication")

		once := sync.Once{}
		wg := sync.WaitGroup{}
		wg.Add(1)
		go s.Subscribe(ctx, func(evt events.Event) {
			switch evt.(type) {
			case *stores.EventReplicated, *stores.EventLoad, *stores.EventWrite, *stores.EventReady:
				println("Replicated or ready")
				members, err := s.ListMembers()
				if err != nil {
					panic(err)
				}

				listMembers(s)

				for _, m := range members {
					if m.Device.Equals(inviterDevicePubKey) {
						once.Do(func() {
							println("inviter found in store", base64.StdEncoding.EncodeToString(invitation.InviterDevicePubKey))
							wg.Done()
						})
					}
				}
			}
		})

		wg.Wait()

		println("redeeming invitation issued by", base64.StdEncoding.EncodeToString(invitation.InviterDevicePubKey))
	}

	_, err = s.RedeemInvitation(ctx, member, device, invitation)
	if err != nil {
		panic(err)
	}

	listMembers(s)
	issueNewInvitation(device, g)

	s.Subscribe(ctx, func(e events.Event) {
		switch e.(type) {
		case *stores.EventReplicated:
			println("New member detected")
			listMembers(s)
			issueNewInvitation(device, g)
			break
		}
	})

	<-ctx.Done()
}

func main() {
	var (
		i   *group.Invitation
		err error
	)

	create := len(os.Args) == 1

	if create {
		println("Creating a new group")
		_, i, err = group.New()
		if err != nil {
			panic(err)
		}
	} else {
		println("Joining an existing group")
		// Read invitation (as base64 on stdin)
		iB64, err := base64.StdEncoding.DecodeString(os.Args[1])
		if err != nil {
			panic(err)
		}

		i = &group.Invitation{}
		err = i.Unmarshal(iB64)
		if err != nil {
			panic(err)
		}
	}

	mainLoop(i, create)
}
