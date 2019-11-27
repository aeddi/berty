package orbitutil

import (
	"context"
	"encoding/hex"

	"berty.tech/go/internal/group"

	"berty.tech/go-orbit-db/accesscontroller"

	"berty.tech/go-orbit-db/stores"

	"berty.tech/go-ipfs-log/identityprovider"
	orbitdb "berty.tech/go-orbit-db"
	"berty.tech/go-orbit-db/address"
	"berty.tech/go-orbit-db/iface"
	"berty.tech/go/pkg/errcode"
	coreapi "github.com/ipfs/interface-go-ipfs-core"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/pkg/errors"
)

const groupIDKey = "group_id"
const memberStoreType = "member_store"

func (s *GroupHolder) getGroup(groupID string) (*GroupContext, error) {
	g, ok := s.groups[groupID]

	if !ok {
		return nil, errcode.ErrGroupMemberMissingSecrets
	}

	return g, nil
}

func (s *GroupHolder) getGroupFromOptions(options *iface.NewStoreOptions) (*GroupContext, error) {
	groupIDs, err := options.AccessController.GetAuthorizedByRole(groupIDKey)
	if err != nil {
		return nil, errcode.TODO.Wrap(err)
	}

	if len(groupIDs) != 1 {
		return nil, errcode.ErrInvalidInput
	}

	return s.getGroup(groupIDs[0])
}

func (s *GroupHolder) AddGroup(ctx context.Context, o orbitdb.OrbitDB, g *group.Group, options *orbitdb.CreateDBOptions) (*GroupContext, error) {
	gc := &GroupContext{Group: g}

	if err := s.setGroup(gc); err != nil {
		return nil, errcode.TODO.Wrap(err)
	}

	if options == nil {
		options = &orbitdb.CreateDBOptions{}
	}
	options.Create = boolPtr(true)

	groupID, err := g.GroupIDAsString()
	if err != nil {
		return nil, errcode.TODO.Wrap(err)
	}

	signingKeyBytes, err := g.SigningKey.GetPublic().Raw()
	if err != nil {
		return nil, errcode.TODO.Wrap(err)
	}

	if options.AccessController == nil {
		options.AccessController = accesscontroller.NewSimpleManifestParams("simple", map[string][]string{
			"write":    {hex.EncodeToString(signingKeyBytes)},
			groupIDKey: {groupID},
		})
	}

	if err := s.keyStore.SetKey(g.SigningKey); err != nil {
		return nil, errcode.TODO.Wrap(err)
	}

	options.Keystore = s.keyStore
	options.Identity, err = identityprovider.CreateIdentity(&identityprovider.CreateIdentityOptions{
		Type:     IdentityType,
		Keystore: s.keyStore,
		ID:       hex.EncodeToString(signingKeyBytes),
	})
	if err != nil {
		return nil, errcode.TODO.Wrap(err)
	}

	gc.MemberStore, err = s.newMemberStore(ctx, o, gc, *options)
	if err != nil {
		return nil, errcode.TODO.Wrap(err)
	}

	return gc, nil
}

// newMemberStore Creates or opens a MemberStore
func (s *GroupHolder) newMemberStore(ctx context.Context, o orbitdb.OrbitDB, gc *GroupContext, options orbitdb.CreateDBOptions) (MemberStore, error) {
	groupID, err := gc.Group.GroupIDAsString()
	if err != nil {
		return nil, errcode.TODO.Wrap(err)
	}

	options.StoreType = stringPtr(memberStoreType)

	store, err := o.Open(ctx, groupID, &options)
	if err != nil {
		return nil, errors.Wrap(err, "unable to open database")
	}

	memberStore, ok := store.(*memberStore)
	if !ok {
		return nil, errors.New("unable to cast store to member store")
	}

	memberStore.groupContext = gc

	return memberStore, nil
}

func (s *GroupHolder) memberStoreConstructor(ctx context.Context, ipfs coreapi.CoreAPI, identity *identityprovider.Identity, addr address.Address, options *iface.NewStoreOptions) (iface.Store, error) {
	g, err := s.getGroupFromOptions(options)
	if err != nil {
		return nil, errcode.TODO.Wrap(err)
	}
	options.Index = NewMemberStoreIndex(g)

	store := &memberStore{}
	err = store.InitBaseStore(ctx, ipfs, identity, addr, options)
	if err != nil {
		return nil, errors.Wrap(err, "unable to initialize base store")
	}

	return store, nil
}

type GroupContext struct {
	Group       *group.Group
	MemberStore MemberStore
	//secretStore SecretStore
}

type GroupHolder struct {
	groups          map[string]*GroupContext
	groupsSigPubKey map[string]crypto.PubKey
	keyStore        *BertySignedKeyStore
}

// NewGroupHolder creates a new GroupHolder which will hold the groups
func NewGroupHolder() (*GroupHolder, error) {
	secretHolder := &GroupHolder{
		groups:          map[string]*GroupContext{},
		groupsSigPubKey: map[string]crypto.PubKey{},
		keyStore:        NewBertySignedKeyStore(),
	}

	// TODO: we can only have a single instance of GroupHolder, otherwise secrets won't be properly retrieved
	stores.RegisterStore(memberStoreType, secretHolder.memberStoreConstructor)
	if err := identityprovider.AddIdentityProvider(NewBertySignedIdentityProviderFactory(secretHolder.keyStore)); err != nil {
		return nil, errcode.TODO.Wrap(err)
	}

	return secretHolder, nil
}

// setGroup registers a new group
func (s *GroupHolder) setGroup(g *GroupContext) error {
	groupID, err := g.Group.GroupIDAsString()
	if err != nil {
		return errcode.TODO.Wrap(err)
	}

	s.groups[groupID] = g

	if err = s.SetGroupSigPubKey(groupID, g.Group.SigningKey.GetPublic()); err != nil {
		return errcode.TODO.Wrap(err)
	}

	return nil
}

// SetGroupSigPubKey registers a new group signature pubkey, mainly used to
// replicate a store data without needing to access to its content
func (s *GroupHolder) SetGroupSigPubKey(groupID string, pubKey crypto.PubKey) error {
	if pubKey == nil {
		return errcode.ErrInvalidInput
	}

	s.groupsSigPubKey[groupID] = pubKey

	return nil
}
