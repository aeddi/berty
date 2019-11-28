package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	mrand "math/rand"
	"os"
	"path"
	"sync"
	"time"

	"berty.tech/go-orbit-db/stores"

	"berty.tech/go-orbit-db/events"
	"berty.tech/go/internal/orbitutil"
	"github.com/libp2p/go-libp2p-core/crypto"

	orbitdb "berty.tech/go-orbit-db"

	"berty.tech/go/internal/ipfsutil"

	"berty.tech/go/internal/group"
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

func listMembers(s orbitutil.MemberStore) {
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

	secretHolder, err := orbitutil.NewGroupHolder()
	if err != nil {
		panic(err)
	}

	p := path.Join(os.TempDir(), base64.StdEncoding.EncodeToString(invitation.InvitationPrivKey))

	odb, err := orbitdb.NewOrbitDB(ctx, api, &orbitdb.NewOrbitDBOptions{Directory: &p})
	if err != nil {
		panic(err)
	}

	createDB := true

	gc, err := secretHolder.AddGroup(ctx, odb, g, &orbitdb.CreateDBOptions{
		Create:    &createDB,
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

	mrand.Seed(time.Now().UnixNano())
	randomSecret := mrand.Intn(89999) + 10000

	fmt.Println("My secret is:", randomSecret)

	println("Own member key:", base64.StdEncoding.EncodeToString(memberKeyBytes), "device key: ", base64.StdEncoding.EncodeToString(deviceKeyBytes))

	if !create {
		println("Waiting store replication")

		once := sync.Once{}
		wg := sync.WaitGroup{}
		wg.Add(1)
		go gc.MemberStore.Subscribe(ctx, func(evt events.Event) {
			switch evt.(type) {
			case *stores.EventReplicated, *stores.EventLoad, *stores.EventWrite, *stores.EventReady:
				println("Replicated or ready")
				members, err := gc.MemberStore.ListMembers()
				if err != nil {
					panic(err)
				}

				listMembers(gc.MemberStore)

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

	_, err = gc.MemberStore.RedeemInvitation(ctx, member, device, invitation)
	if err != nil {
		panic(err)
	}

	listMembers(gc.MemberStore)
	issueNewInvitation(device, g)

	gc.MemberStore.Subscribe(ctx, func(e events.Event) {
		switch e.(type) {
		case *stores.EventReplicated:
			println("New member detected")
			listMembers(gc.MemberStore)
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
