package disklog

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/jimsnab/go-lane"
)

type (
	DiskLogServer interface {
		// Starts listening for connections
		StartServer(endpoint string, port int) error

		// Initiates server termination, if it is running.
		StopServer() error

		// Waits for the server to stop
		WaitForTermination()

		// Returns the server address
		ServerAddr() string
	}

	diskLogServer struct {
		mu              sync.Mutex
		started         bool
		l               lane.Lane
		server          net.Listener
		exitSaver       chan struct{}
		saverTerminated chan struct{}
		canExit         chan struct{}
		terminating     bool
		port            int
		iface           string
		cxns            *clientCxns
		diskLogger      *diskLogger
	}
)

func NewDiskLogServer(l lane.Lane, persistPath string) (server DiskLogServer, err error) {
	server = &diskLogServer{
		l:    l,
		cxns: newClientCxns(l),
		diskLogger: newDiskLogger(l, persistPath),
	}
	return
}


func (dls *diskLogServer) StartServer(endpoint string, port int) (err error) {
	dls.mu.Lock()
	defer dls.mu.Unlock()

	if dls.started {
		panic("already started")
	}

	if port != 0 {
		dls.port = port
	} else {
		dls.port = 6801
	}

	if endpoint != "" {
		dls.iface = endpoint
	}

	// launch termination monitiors
	dls.canExit = make(chan struct{})

	// start accepting connections and processing them
	err = dls.startServer()
	if err != nil {
		return err
	}
	dls.started = true

	return nil
}

func (dls *diskLogServer) StopServer() error {
	// ensure only one termination
	dls.mu.Lock()
	if !dls.started {
		dls.mu.Unlock()
		return fmt.Errorf("not started")
	}

	isTerminating := dls.terminating
	dls.terminating = true
	dls.mu.Unlock()

	if !isTerminating {
		go func() { dls.onTerminate() }()
	}

	return nil
}

func (dls *diskLogServer) onTerminate() {
	if dls.server != nil {
		// close the server and wait for all active connections to finish
		dls.l.Tracef("closing server")
		dls.server.Close()

		dls.mu.Lock()
		for _, cxn := range dls.cxns {
			dls.l.Tracef("closing connection %s <-> %s", cxn.LocalAddr().String(), cxn.RemoteAddr().String())
			cxn.Close()
		}
		dls.cxns = []net.Conn{}
		dls.mu.Unlock()

		dls.l.Infof("waiting for any open request connections to complete")
		requestAllCxnClose()
		waitForAllCxnClose()
		dls.l.Infof("termination of %s completed", dls.server.Addr().String())
	}

	// stop the periodic saver (if running)
	if dls.exitSaver != nil {
		dls.l.Tracef("closing database saver")
		dls.exitSaver <- struct{}{}
		<-dls.saverTerminated
		dls.l.Tracef("database saver closed")
	}

	dls.canExit <- struct{}{}
}

func (dls *diskLogServer) startServer() error {
	// establish socket service
	var err error

	if dls.iface == "" {
		dls.iface = fmt.Sprintf(":%d", dls.port)
	} else {
		dls.iface = fmt.Sprintf("%s:%d", dls.iface, dls.port)
	}

	dls.server, err = net.Listen("tcp", dls.iface)
	if err != nil {
		dls.l.Errorf("error listening: %s", err.Error())
		return err
	}
	dls.l.Infof("listening on %s", dls.server.Addr().String())

	go func() {
		// accept connections and process commands
		for {
			connection, err := dls.server.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					dls.l.Errorf("accept error: %s", err)
				}
				break
			}
			dls.mu.Lock()
			dls.cxns = append(dls.cxns, connection)
			dls.mu.Unlock()
			dls.l.Infof("client connected: %s", connection.RemoteAddr().String())
			newClientCxn(dls.l, connection, dls.diskLogger)
		}
	}()

	return nil
}

func (dls *diskLogServer) WaitForTermination() {
	// wait for server to quiesque
	<-dls.canExit
	dls.l.Info("finished serving requests")
}

func (dls *diskLogServer) ServerAddr() string {
	dls.mu.Lock()
	defer dls.mu.Unlock()

	if dls.server == nil {
		return ""
	}

	return dls.server.Addr().String()
}
