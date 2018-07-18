package gossip

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/assert"
)

func TestGossip(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlError), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	howMany := 1000
	kNodes := 3

	system := &NodeSystem{
		GNodes:            make([]*GNode, howMany),
		KNodes:            kNodes,
		ArtificialLatency: 500,
	}

	for i := 0; i < howMany; i++ {
		node := NewGNode(strconv.Itoa(i))
		node.NodeSystem = system
		system.GNodes[i] = node
		node.Index = i
		node.Start()
	}

	defer func() {
		for i := 0; i < howMany; i++ {
			system.GNodes[i].Stop()
		}
	}()

	respChan := make(chan *WireResponse)

	msg := []byte("hi")
	//hsh := crypto.Keccak256(msg)

	req := &WireRequest{
		NodeId:       "initial",
		ResponseChan: respChan,
		Message:      msg,
	}

	node := system.GNodes[randInt(len(system.GNodes))]

	node.Incoming <- req
	resp := <-respChan
	assert.False(t, resp.Accepted)

	now := time.Now()
	allDone := 0

	for allDone < howMany {
		for i := 0; i < howMany; i++ {
			if system.GNodes[i].Accepted {
				allDone += 1
			}
		}
		log.Debug("accepted", allDone)
		if time.Now().Unix()-now.Unix() > 15 {
			t.Errorf("timeout")
			break
		}

		if allDone < howMany {
			time.Sleep(1 * time.Second)
			allDone = 0
		}

	}
	stopped := time.Now()

	log.Info("stats", "encodes", countEncodes, "accepted", allDone, "start", now, "stop", stopped, "elapsed", time.Duration(stopped.UnixNano()-now.UnixNano()), "perEncode", time.Duration((stopped.UnixNano()-now.UnixNano())/int64(countEncodes)))
	//for i := 0; i < howMany; i++ {
	//	node = system.GNodes[i]
	//	if !node.Accepted {
	//		log.Debug("not accepted", "node", i)
	//		t.Logf("node %d not accepted", i)
	//	}
	//	log.Info("node stats", "node", node.Index, "receivedCount", node.ReceivedCount, "gossipCount", node.GossipCount)
	//}
}
