package disklog

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/jimsnab/go-lane"
)

const cBufferSize = 32 * 1024
const cMaxMessageSize = 128 * 1024

const (
	csActive cxnState = iota
	csTerminating
	csTerminated
)

type (
	cxnState int

	// clientCxn holds state about the socket connection.
	clientCxn struct {
		mu          sync.Mutex // synchronizes access to waiting, closing flags
		l           lane.Lane
		cxn         net.Conn
		state     cxnState
		inbound     []byte
		respVersion int
		closed chan struct{}
		received     []byte
		dl          *diskLogger
	}
)

func newClientCxn(l lane.Lane, cxn net.Conn, dl *diskLogger) *clientCxn {
	cc := &clientCxn{
		cxn: cxn,
		l: l.Derive(),
		closed: make(chan struct{}),
		dl: dl,
	}

	go cc.run()
	return cc
}

// request connection close
func (cc *clientCxn) RequestClose() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.state == csActive {
		cc.state = csTerminating
		cc.cxn.Close()				// close the socket
	}
}

func (cc *clientCxn) IsCloseRequested() bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.state != csActive
}

// block until the connection go routine has completed
func (cc *clientCxn) WaitForClose() {
	// don't wait on channel if already closed
	cc.mu.Lock()
	if cc.state == csTerminated {
		cc.mu.Unlock()
		return
	}
	cc.mu.Unlock()

	// block until go routine signals close
	<- cc.closed
	
	// go routine sets cc.state to csTerminated
}

func (cc *clientCxn) run() {
	for {
		b := make([]byte, cBufferSize)
		n, err := cc.cxn.Read(b)
		if err != nil {
			cc.mu.Lock()
			if cc.state == csActive {
				cc.l.Errorf("failed to read from %s: %v", cc.ClientAddr(), err)
			}
			cc.mu.Unlock()
			break
		}

		cc.received = append(cc.received, b[:n]...)

		for {
			processed, err := cc.consume()
			if err != nil {
				cc.l.Errorf("failed to process %s: %v", cc.ClientAddr(), err)
				cc.cxn.Close()
			}

			if processed == 0 {
				break
			}

			cc.received = cc.received[processed:]
		}
	}

	cc.mu.Lock()
	cc.state = csTerminated
	cc.mu.Unlock()
}

func (cc *clientCxn) consume() (processed int, err error) {
	if len(cc.received) < 4 {
		return
	}

	size := binary.BigEndian.Uint32(cc.received[:4])
	if size > cMaxMessageSize {
		err = fmt.Errorf("stream input length %08X is invalid", size);
		return
	}

	if len(cc.received) < (4 + int(size)) {
		return
	}

	s := string(cc.received[4:4 + size])
	cutPoint := strings.Index(s, "\t")
	if cutPoint < 0 {
		err = fmt.Errorf("stream input requires <logname>\\t<message> schema")
		return
	}

	err = cc.dl.processMessage(s[:cutPoint], s[cutPoint + 1:])
	if err != nil {
		return
	}

	processed = int(size) + 4
	return
}

func (cc *clientCxn) ServerAddr() string {
	return cc.cxn.LocalAddr().String()
}

func (cc *clientCxn) ClientAddr() string {
	return cc.cxn.RemoteAddr().String()
}
