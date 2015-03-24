package coconet

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/dedis/crypto/abstract"
)

// Default timeout for any network operation
const DefaultGoTimeout time.Duration = 500 * time.Millisecond

var TimeoutError error = errors.New("Network timeout error")

// HostNode is a simple implementation of Host that does not specify the
// communication medium (goroutines/channels, network nodes/tcp, ...).
type GoHost struct {
	name string // the hostname

	plock  sync.Mutex
	parent Conn // the Peer representing parent, nil if root

	childLock   sync.Mutex
	children    []string // a list of unique peers for each hostname
	childrenMap map[string]Conn
	peers       map[string]Conn
	dir         *GoDirectory

	rlock sync.Mutex
	ready map[Conn]bool

	mupk   sync.RWMutex
	Pubkey abstract.Point // own public key

	mutimeout sync.Mutex
	timeout   time.Duration // general timeout for any network operation

	pool  sync.Pool
	suite abstract.Suite
}

func (h *GoHost) SetSuite(suite abstract.Suite) {
	h.suite = suite
}

func (h *GoHost) GetDirectory() *GoDirectory {
	return h.dir
}

func (h *GoHost) DefaultTimeout() time.Duration {
	h.mutimeout.Lock()
	t := DefaultGoTimeout
	h.mutimeout.Unlock()
	return t
}

func (h *GoHost) SetTimeout(t time.Duration) {
	h.mutimeout.Lock()
	h.timeout = t
	h.mutimeout.Unlock()
}

func (h *GoHost) Timeout() time.Duration {
	var t time.Duration
	h.mutimeout.Lock()
	t = h.timeout
	h.mutimeout.Unlock()
	return t
}

// NewHostNode creates a new HostNode with a given hostname.
func NewGoHost(hostname string, dir *GoDirectory) *GoHost {
	h := &GoHost{name: hostname,
		children:    make([]string, 0),
		childrenMap: make(map[string]Conn),
		peers:       make(map[string]Conn),
		dir:         dir}
	h.mutimeout.Lock()
	h.timeout = DefaultGoTimeout
	h.mutimeout.Unlock()
	h.rlock = sync.Mutex{}
	h.ready = make(map[Conn]bool)
	return h
}

func (h *GoHost) PubKey() abstract.Point {
	h.mupk.RLock()
	pk := h.Pubkey
	h.mupk.RUnlock()
	return pk
}

func (h *GoHost) SetPubKey(pk abstract.Point) {
	h.mupk.Lock()
	h.Pubkey = pk
	h.mupk.Unlock()
}

func (h *GoHost) Connect() error {
	return nil
}

func (h *GoHost) Listen() error {
	h.childLock.Lock()
	children := make([]string, len(h.children))
	copy(children, h.children)
	h.childLock.Unlock()

	suite := h.suite
	// each conn should have a Ready() bool, SetReady(bool)
	for _, c := range children {
		go func(c string) {
			pubkey := suite.Point()

			h.rlock.Lock()
			conn := h.peers[c]
			h.rlock.Unlock()

			e := conn.Get(pubkey)
			if e != nil {
				log.Fatal("unable to get pubkey from child")
			}
			conn.SetPubKey(pubkey)
			h.rlock.Lock()
			h.ready[conn] = true
			h.rlock.Unlock()
			// fmt.Println("connection with child established")
		}(c)
	}
	return nil
}

// AddParent adds a parent node to the HostNode.
func (h *GoHost) AddParent(c string) {
	if _, ok := h.peers[c]; !ok {
		h.peers[c], _ = NewGoConn(h.dir, h.name, c)
	}
	h.plock.Lock()
	h.peers[c].Put(h.PubKey()) // publick key should be put here first
	// only after putting pub key allow it to be accessed like parent
	h.parent, _ = h.peers[c]
	h.plock.Unlock()
}

// AddChildren variadically adds multiple Peers as children to the HostNode.
// Only unique children will be stored.
func (h *GoHost) AddChildren(cs ...string) {
	for _, c := range cs {
		if _, ok := h.peers[c]; !ok {
			h.peers[c], _ = NewGoConn(h.dir, h.name, c)
		}
		// don't allow children to be accessed before adding them
		h.children = append(h.children, c)
		h.childrenMap[c] = h.peers[c]
	}
}

func (h *GoHost) Close() {}

func (h *GoHost) NChildren() int {
	return len(h.children)
}

// Name returns the hostname of the HostNode.
func (h *GoHost) Name() string {
	return h.name
}

// IsRoot returns true if the HostNode is the root of it's tree (if it has no
// parent).
func (h *GoHost) IsRoot() bool {
	h.plock.Lock()
	defer h.plock.Unlock()
	return h.parent == nil
}

// Peers returns the list of peers as a mapping from hostname to Conn
func (h *GoHost) Peers() map[string]Conn {
	return h.peers
}

func (h *GoHost) Children() map[string]Conn {
	return h.childrenMap
}

// AddPeers adds the list of peers
func (h *GoHost) AddPeers(cs ...string) {
	for _, c := range cs {
		h.peers[c], _ = NewGoConn(h.dir, h.name, c)
	}
}

// WaitTick waits for a random amount of time.
// XXX should it wait for a network change
func (h *GoHost) WaitTick() {
	time.Sleep(1 * time.Second)
}

// TODO(dyv): following methods will have to be rethought with network failures and
// latency in mind. What happens during GetDown if one node serves up old
// responses after a certain timeout?

// PutUp sends a message (an interface{} value) up to the parent through
// whatever 'network' interface the parent Peer implements.
func (h *GoHost) PutUp(data BinaryMarshaler) error {
	// defer fmt.Println(h.Name(), "done put up", h.parent)
	// fmt.Printf(h.Name(), "PUTTING UP up:%#v", data)
	return h.parent.Put(data)
}

// GetUp gets a message (an interface{} value) from the parent through
// whatever 'network' interface the parent Peer implements.
func (h *GoHost) GetUp(data BinaryUnmarshaler) error {
	// fmt.Println("GETTING UP from up")
	return h.parent.Get(data)
}

// PutDown sends a message (an interface{} value) up to all children through
// whatever 'network' interface each child Peer implements.
func (h *GoHost) PutDown(data []BinaryMarshaler) error {
	// fmt.Println("PUTTING DOWN")
	if len(data) != len(h.children) {
		panic("number of messages passed down != number of children")
	}
	// Try to send the message to all children
	// If at least on of the attempts fails, return a non-nil error
	var err error
	i := 0
	for _, c := range h.children {
		h.rlock.Lock()
		conn := h.peers[c]
		h.rlock.Unlock()
		if e := conn.Put(data[i]); e != nil {
			err = e
		}
		i++
	}
	return err
}

func (h *GoHost) whenReadyGet(c Conn, data BinaryUnmarshaler) error {
	// defer fmt.Println(h.Name(), "returned ready channel for", c.Name(), c)
	for {
		h.rlock.Lock()
		isReady := h.ready[c]
		h.rlock.Unlock()

		if isReady {
			// fmt.Println(h.Name(), "is ready")
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	return c.Get(data)
}

// GetDown gets a message (an interface{} value) from all children through
// whatever 'network' interface each child Peer implements.
func (h *GoHost) GetDown() (chan NetworkMessg, chan error) {
	// fmt.Println("GETTING DOWN")
	var chmu sync.Mutex
	ch := make(chan NetworkMessg, 1)
	errch := make(chan error, 1)

	// start children threads
	go func() {
		for i, c := range h.children {
			go func(i int, c string) {
				h.rlock.Lock()
				conn := h.peers[c]
				h.rlock.Unlock()

				for {
					data := h.pool.Get().(BinaryUnmarshaler)
					e := h.whenReadyGet(conn, data)

					chmu.Lock()
					ch <- NetworkMessg{Data: data, From: c} // this should be copy of data[i]
					errch <- e
					chmu.Unlock()

				}
			}(i, c)
		}
	}()

	return ch, errch
}

func (h *GoHost) Pool() sync.Pool {
	return h.pool
}

func (h *GoHost) SetPool(p sync.Pool) {
	h.pool = p
}
