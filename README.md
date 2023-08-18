# disklog-go
Aggregates logs from many connections to a single disk

## About
This library provides a server that accepts many client connections and aggregates
logging activity into common files. It is useful when you have multiple instances
of a service and all you want for logging is a file you can tail, grep, etc.

`github.com/jimsnab/go-lane` provides `NetLogLane`

## Protocol
The client connects to the server and sends messages in this simple format:

`<length-uint32 big endian><log-name>\t<message>`

Where:

* *length* provides the number of bytes in the message, excluding the four bytes for the length itself
* *log-name* a short text name of the log (e.g., "access", "my-service", etc.)
* *message* the text to write to the log; a final line ending \n will be added if it is not included

## Use
This library implements the server. A host program is required.

```
package main

import (
    "context"

	disklog "github.com/jimsnab/disklog-go"    
	"github.com/jimsnab/go-lane"    
)

func main() {
    l := lane.NewLogLane(context.Background())  // for this service's logging, not the logs written to disk

    srv := disklog.NewDiskLogServer(l, "/var/log/disklog")

    err := srv.StartServer("localhost", 0) // uses port 6801 by default
	if err != nil {
        l.Fatal(err)
	}

    // graceful termination
    killSignalMonitor(l, srv)

    srv.WaitForTermination()
}

func killSignalMonitor(l lane.Lane, srv tscmdsrv.TreeStoreCmdLineServer) {
	// register a graceful termination handler
	sigs := make(chan os.Signal, 10)
	signal.Notify(sigs, os.Interrupt)

	go func() {
		sig := <-sigs
		l.Infof("termination %s signaled for %s", sig, srv.ServerAddr())
		srv.StopServer()
	}()
}
```
