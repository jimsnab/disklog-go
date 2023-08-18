package disklog

import (
	"net"
	"sync"

	"github.com/jimsnab/go-lane"
)

type (
	clientCxns struct {
		sync.Mutex
		l lane.Lane
		cxns            []*clientCxn
	}
)

func newClientCxns(l lane.Lane) (cxns *clientCxns) {
	return &clientCxns{
		l: l,
		cxns: []*clientCxn{},
	}
}

func (ccs *clientCxns) processAllClients(op func(cxn *clientCxn)) {
	for _, cxn := range ccs.cxns {
		if !cxn.IsCloseRequested() {
			op(cxn)
		}
	}
}

func (ccs *clientCxns) terminateAll() {
	ccs.Lock()
	defer ccs.Unlock()

	ccs.processAllClients(func(cxn *clientCxn) {cxn.RequestClose()})
	ccs.processAllClients(func(cxn *clientCxn) {cxn.WaitForClose()})

	for _, cxn := range ccs.cxns {
		ccs.l.Tracef("closing connection %s <-> %s", cxn.LocalAddr().String(), cxn.RemoteAddr().String())
		cxn.Close()
	}
	ccs.cxns = []net.Conn{}
}