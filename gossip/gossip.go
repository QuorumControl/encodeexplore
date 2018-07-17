package gossip

import (
	"crypto/rand"

	"github.com/ethereum/go-ethereum/log"

	"sync"

	"math/big"

	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multihash"
)

func init() {
	cbornode.RegisterCborType(WireRequest{})
	cbornode.RegisterCborType(WireResponse{})
	cbornode.RegisterCborType(GossipSig{})
}

var countEncodes = 0

var threshold = float64(2) / float64(3)

type NodeSystem struct {
	GNodes            []*GNode
	KNodes            int
	ArtificialLatency int
}

type GNode struct {
	Id                string
	Accepted          bool
	MessageHash       []byte
	Message           []byte
	CurrentSig        *GossipSig
	NodeSystem        *NodeSystem
	Incoming          chan *WireRequest
	Index             int
	stopChan          chan bool
	gossipChan        chan bool
	signatureLock     *sync.Mutex
	ReceivedCount     int
	GossipCount       int
	IncomingBandwidth int64
	OutgoingBandwidth int64
}

func NewGNode(id string) *GNode {
	return &GNode{
		Id:            id,
		Incoming:      make(chan *WireRequest, 10000),
		stopChan:      make(chan bool, 1),
		gossipChan:    make(chan bool, 1),
		signatureLock: &sync.Mutex{},
	}
}

type WireRequest struct {
	NodeId       string
	Message      []byte
	Signature    GossipSig
	ResponseChan chan *WireResponse `refmt:"-" json:"-" cbor:"-"`
}

type WireResponse struct {
	NodeId      string
	Accepted    bool
	Signature   GossipSig
	MessageHash []byte
}

func (gn *GNode) Start() {
	go func() {
		for {
			select {
			case wr := <-gn.Incoming:
				go func() {
					resp, err := gn.OnMessage(wr)
					time.Sleep(time.Duration(gn.NodeSystem.ArtificialLatency) * time.Millisecond)
					if err != nil {
						log.Trace("", "node", gn.Id, "err", err)
						wr.ResponseChan <- nil
					} else {
						log.Trace("responding", "node", gn.Id, "to", wr.NodeId, "sigCount", len(resp.Signature.Signatures))
						wr.ResponseChan <- resp
					}
				}()
			case <-gn.gossipChan:
				go func() {

					if len(gn.CurrentSig.Missing(gn.NodeSystem)) == 0 {
						return
					}

					gn.doGossip()

					if len(gn.CurrentSig.Missing(gn.NodeSystem)) == 0 {
						gn.Accepted = true
					}
					<-time.After(200 * time.Millisecond)
					gn.gossipChan <- true
				}()

			case <-gn.stopChan:
				break
			}
		}
	}()
}

func randNode(missing map[string]*GNode) *GNode {
	i := randInt(len(missing))
	for _, node := range missing {
		if i == 0 {
			return node
		}
		i--
	}
	panic("never")
}

func (gn *GNode) doGossip() {
	gn.GossipCount++
	missing := gn.CurrentSig.Missing(gn.NodeSystem)
	log.Trace("missing", "node", gn.Id, "missingCount", len(missing))
	if len(missing) > 0 {
		// do the gossip
		responses := make(map[string]chan *WireResponse)

		for i := 0; i < min(gn.NodeSystem.KNodes, len(missing)); i++ {
			var node *GNode
			for node == nil || node.Id == gn.Id {
				node = gn.NodeSystem.GNodes[randInt(len(gn.NodeSystem.GNodes))]
			}
			log.Trace("gossipping", "node", gn.Id, "to", node.Id)
			respChan := make(chan *WireResponse, 1)
			req := &WireRequest{
				NodeId:       gn.Id,
				Message:      gn.Message,
				Signature:    *gn.CurrentSig,
				ResponseChan: respChan,
			}
			countEncodes++
			//start := time.Now()
			//cborNode, _ := cbornode.WrapObject(req, multihash.SHA2_256, -1)
			//gn.OutgoingBandwidth += int64(len(cborNode.RawData()))
			//log.Info("encode", "took", time.Now().Sub(start))

			node.Incoming <- req
			responses[node.Id] = respChan
		}

		for nodeId, respChan := range responses {
			resp := <-respChan
			log.Trace("received", "node", gn.Id, "from", nodeId, "signatureCount", len(resp.Signature.Signatures))
			if resp == nil {
				log.Error("error gossiping", "node", gn.Id)
			}

			err := gn.onNewSig(gn.Message, &resp.Signature)
			if err != nil {
				log.Error("error newSig", "node", gn.Id, "err", err)
			}
		}
	} else {
		gn.Accepted = true
		return
	}

	if 1.0-(float64(len(gn.CurrentSig.Missing(gn.NodeSystem)))/float64(len(gn.NodeSystem.GNodes))) >= threshold {
		log.Debug("moving to accepted state", "node", gn.Id)
		gn.Accepted = true
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func randInt(max int) int {
	bigInt, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		log.Error("error reading rand", "err", err)
	}
	return int(bigInt.Int64())
}

func (gn *GNode) Stop() {
	gn.stopChan <- true
}

func (gn *GNode) onNewSig(msg []byte, messageSig *GossipSig) error {
	gn.signatureLock.Lock()
	defer gn.signatureLock.Unlock()
	if gn.CurrentSig == nil {
		gn.Message = msg

		hsh := crypto.Keccak256(msg)
		gn.MessageHash = hsh

		sig := hsh

		signatures := make([][]byte, len(gn.NodeSystem.GNodes))
		signatures[gn.Index] = sig
		gn.CurrentSig = &GossipSig{
			Signatures: signatures,
		}
		gn.gossipChan <- true
	}
	if messageSig != nil {
		gn.CurrentSig = MergeGossipSigs(gn.CurrentSig, messageSig)
	}

	return nil
}

func (gn *GNode) OnMessage(wr *WireRequest) (*WireResponse, error) {
	gn.ReceivedCount++
	log.Debug("onMessage", "node", gn.Id, "from", wr.NodeId, "signatureCount", len(wr.Signature.Signatures))
	gn.onNewSig(wr.Message, &wr.Signature)

	countEncodes++
	start := time.Now()
	cborNode, _ := cbornode.WrapObject(wr, multihash.SHA2_256, -1)
	gn.IncomingBandwidth += int64(len(cborNode.RawData()))
	log.Info("decode", "took", time.Now().Sub(start))

	if 1.0-(float64(len(gn.CurrentSig.Missing(gn.NodeSystem)))/float64(len(gn.NodeSystem.GNodes))) >= threshold {
		log.Debug("moving to accepted state", "node", gn.Id)
		gn.Accepted = true
	}

	resp := &WireResponse{
		NodeId:      gn.Id,
		Accepted:    gn.Accepted,
		Signature:   *gn.CurrentSig.Subtract(&wr.Signature),
		MessageHash: gn.MessageHash,
	}

	return resp, nil
}
