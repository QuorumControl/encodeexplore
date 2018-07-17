package gossip

type GossipSig struct {
	Signatures [][]byte `refmt:"signature,omitempty" json:"signature,omitempty" cbor:"signature,omitempty"`
}

func (gs *GossipSig) Missing(sys *NodeSystem) map[int]*GNode {
	missing := make(map[int]*GNode)
	for i, sig := range gs.Signatures {
		if len(sig) == 0 {
			node := sys.GNodes[i]
			missing[i] = node
		}
	}
	return missing
}

func MergeGossipSigs(origGs *GossipSig, newGs *GossipSig) *GossipSig {

	if len(newGs.Signatures) == 0 {
		return origGs
	}

	newGossipSig := &GossipSig{
		Signatures: make([][]byte, len(origGs.Signatures)),
	}

	for i, sig := range origGs.Signatures {
		if len(sig) > 0 {
			newGossipSig.Signatures[i] = sig
		} else {
			if len(newGs.Signatures[i]) > 0 {
				newGossipSig.Signatures[i] = newGs.Signatures[i]
			}
		}
	}

	return newGossipSig
}

func (gs *GossipSig) IsEmpty() bool {
	for _, sig := range gs.Signatures {
		if len(sig) > 0 {
			return true
		}
	}
	return false
}

func (gs *GossipSig) Subtract(other *GossipSig) *GossipSig {
	if len(other.Signatures) == 0 {
		return gs
	}

	newGossipSig := &GossipSig{
		Signatures: make([][]byte, len(gs.Signatures)),
	}

	for i, sig := range gs.Signatures {
		if len(other.Signatures[i]) > 0 {
			continue
		}
		newGossipSig.Signatures[i] = sig
	}
	return newGossipSig
}
