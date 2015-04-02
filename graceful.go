package graceful

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var StoppedError = errors.New("Listener stopped")

// tcpStoppableListener acts like the tcpKeepAliveListener
// It also has a stop channel to indicate listener should shutdown
type StoppableListener struct {
	*net.TCPListener
	stop chan int
}
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func New(l net.Listener) (*StoppableListener, error) {
	tcpL, ok := l.(*net.TCPListener)

	if !ok {
		return nil, errors.New("Cannot Wrap Listener")
	}
	retval := &StoppableListener{}
	retval.TCPListener = tcpL
	retval.stop = make(chan int)

	return retval, nil
}
func (sl StoppableListener) Accept() (c net.Conn, err error) {
	for {
		// Wait up to one second for a new connection
		sl.SetDeadline(time.Now().Add(time.Second))
		newConn, err := sl.TCPListener.Accept()

		select {
		case <-sl.stop:
			return nil, StoppedError
		default:
			//If the channel is still open, continue as normal

		}
		if err != nil {
			netErr, ok := err.(net.Error)

			//If this is a timeout, then continue to wait for new connections
			if ok && netErr.Timeout() && netErr.Temporary() {
				continue
			}
		}
		return newConn, err

	}
}

func ListenAndServeTLS(addr string, certFile, keyFile string, handler http.Handler) {
	srv := &http.Server{Addr: addr, Handler: handler}
	if addr == "" {
		addr = ":https"
	}
	config := &tls.Config{}
	if srv.TLSConfig != nil {
		*config = *srv.TLSConfig
	}
	if config.NextProtos == nil {
		config.NextProtos = []string{"http/1.1"}
	}

	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Println(err)
		return
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Println(err)
		return
	}
	sl, err := New(ln)
	if err != nil {
		log.Println(err)
		return
	}

	tlsListener := tls.NewListener(sl, config)
	stop := make(chan os.Signal)
	signal.Notify(stop, syscall.SIGINT)
	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		defer wg.Done()
		srv.Serve(tlsListener)
	}()
	select {
	case signal := <-stop:
		fmt.Printf("Got signal:%v\n", signal)
	}
	fmt.Printf("Stopping listener\n")
	sl.Stop()
	fmt.Printf("Waiting on server\n")
	wg.Wait()
	//return srv.Serve(tlsListener)
}
func (sl *StoppableListener) Stop() {
	close(sl.stop)
}
