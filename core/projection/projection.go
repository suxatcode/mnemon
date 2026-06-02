package projection

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/mnemon-dev/mnemon/core/contract"
	"github.com/mnemon-dev/mnemon/core/kernel"
)

type Projection struct {
	Ref       string
	Digest    string
	Resources []contract.ResourceVersion
	Feedback  []contract.Decision // pull channel (Invariant #8)
}

func Build(s *kernel.Store, refs []contract.ResourceRef, forActor contract.ActorID) Projection {
	var rv []contract.ResourceVersion
	for _, r := range refs {
		v, _ := s.GetVersion(r)
		rv = append(rv, contract.ResourceVersion{Ref: r, Version: v})
	}
	sort.Slice(rv, func(i, j int) bool {
		return string(rv[i].Ref.Kind)+string(rv[i].Ref.ID) < string(rv[j].Ref.Kind)+string(rv[j].Ref.ID)
	})
	h := sha256.New()
	for _, x := range rv {
		fmt.Fprintf(h, "%s:%s:%d;", x.Ref.Kind, x.Ref.ID, x.Version)
	}
	dig := hex.EncodeToString(h.Sum(nil))
	fb, _ := s.DecisionsForActor(forActor)
	return Projection{Ref: "proj_" + dig[:12], Digest: dig, Resources: rv, Feedback: fb}
}
