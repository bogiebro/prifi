package sign_test

import (
	"fmt"
	"log"
	"strconv"
	"testing"
	"time"

	"github.com/dedis/crypto/abstract"
	"github.com/dedis/crypto/edwards/ed25519"
	"github.com/dedis/crypto/nist"
	_ "github.com/dedis/prifi/coco"
	"github.com/dedis/prifi/coco/coconet"
	"github.com/dedis/prifi/coco/sign"
	"github.com/dedis/prifi/coco/test/oldconfig"
)

// NOTE: when announcing must provide round numbers

// Testing suite for signing
// NOTE: when testing if we can gracefully accommodate failures we must:
// 1. Wrap our hosts in FaultyHosts (ex: via field passed in LoadConfig)
// 2. Set out Nodes TesingFailures field to true
// 3. We can Choose at which stage our nodes fail by using SetDeadFor
//    or we can choose to take them off completely via SetDead

//       0
//      /
//     1
//    / \
//   2   3
func TestStaticMerkle(t *testing.T) {
	if err := runStaticTest(sign.MerkleTree); err != nil {
		t.Fatal(err)
	}
}

func TestStaticPubKey(t *testing.T) {
	if err := runStaticTest(sign.PubKey); err != nil {
		t.Fatal(err)
	}
}

func TestStaticFaulty(t *testing.T) {
	faultyNodes := make([]int, 0)
	faultyNodes = append(faultyNodes, 1)

	if err := runStaticTest(sign.PubKey, faultyNodes...); err != nil {
		t.Fatal(err)
	}
}

func runStaticTest(signType sign.Type, faultyNodes ...int) error {
	// Crypto setup
	suite := nist.NewAES128SHA256P256()
	rand := suite.Cipher([]byte("example"))

	// number of nodes for the test
	nNodes := 4
	// create new directory for communication between peers
	dir := coconet.NewGoDirectory()
	// Create Hosts and Peers
	h := make([]coconet.Host, nNodes)

	for i := 0; i < nNodes; i++ {
		hostName := "host" + strconv.Itoa(i)

		if len(faultyNodes) > 0 {
			h[i] = &coconet.FaultyHost{}
			gohost := coconet.NewGoHost(hostName, dir)
			h[i] = coconet.NewFaultyHost(gohost)
		} else {
			h[i] = coconet.NewGoHost(hostName, dir)
		}

	}

	for _, fh := range faultyNodes {
		h[fh].(*coconet.FaultyHost).SetDeadFor("response", true)
	}

	// Create Signing Nodes out of the hosts
	nodes := make([]*sign.Node, nNodes)
	for i := 0; i < nNodes; i++ {
		nodes[i] = sign.NewNode(h[i], suite, rand)
		nodes[i].Type = signType

		h[i].SetPubKey(nodes[i].PubKey)
		// To test the already keyed signing node, uncomment
		// PrivKey := suite.Secret().Pick(rand)
		// nodes[i] = NewKeyedNode(h[i], suite, PrivKey)
	}
	nodes[0].Height = 2
	nodes[1].Height = 1
	nodes[2].Height = 0
	nodes[3].Height = 0
	// Add edges to parents
	h[1].AddParent(h[0].Name())
	h[2].AddParent(h[1].Name())
	h[3].AddParent(h[1].Name())
	// Add edges to children, listen to children
	h[0].AddChildren(h[1].Name())
	h[0].Listen()
	h[1].AddChildren(h[2].Name(), h[3].Name())
	h[1].Listen()

	for i := 0; i < nNodes; i++ {
		if len(faultyNodes) > 0 {
			nodes[i].FailureRate = 1
		}

		go func(i int) {
			// start listening for messages from within the tree
			nodes[i].Listen()
		}(i)
	}

	// Have root node initiate the signing protocol
	// via a simple annoucement
	nodes[0].LogTest = []byte("Hello World")
	return nodes[0].Announce(&sign.AnnouncementMessage{LogTest: nodes[0].LogTest, Round: 0})
}

// Configuration file data/exconf.json
//       0
//      / \
//     1   4
//    / \   \
//   2   3   5
func TestSmallConfigHealthy(t *testing.T) {
	suite := nist.NewAES128SHA256P256()
	if err := runTreeSmallConfig(sign.MerkleTree, suite, 0); err != nil {
		t.Fatal(err)
	}
}

func TestSmallConfigHealthyNistQR512(t *testing.T) {
	suite := nist.NewAES128SHA256QR512()
	if err := runTreeSmallConfig(sign.MerkleTree, suite, 0); err != nil {
		t.Fatal(err)
	}
}

func TestSmallConfigHealthyEd25519(t *testing.T) {
	suite := ed25519.NewAES128SHA256Ed25519(true)
	if err := runTreeSmallConfig(sign.MerkleTree, suite, 0); err != nil {
		t.Fatal(err)
	}
}

func TestSmallConfigFaulty(t *testing.T) {
	faultyNodes := make([]int, 0)
	faultyNodes = append(faultyNodes, 2, 5)
	suite := nist.NewAES128SHA256P256()
	if err := runTreeSmallConfig(sign.MerkleTree, suite, 100, faultyNodes...); err != nil {
		t.Fatal(err)
	}
}

func TestSmallConfigFaulty2(t *testing.T) {
	failureRate := 15
	faultyNodes := make([]int, 0)
	faultyNodes = append(faultyNodes, 1, 2, 3, 4, 5)
	suite := nist.NewAES128SHA256P256()
	if err := runTreeSmallConfig(sign.MerkleTree, suite, failureRate, faultyNodes...); err != nil {
		t.Fatal(err)
	}
}

func runTreeSmallConfig(signType sign.Type, suite abstract.Suite, failureRate int, faultyNodes ...int) error {
	var hostConfig *oldconfig.HostConfig
	var err error
	opts := oldconfig.ConfigOptions{Suite: suite}

	if len(faultyNodes) > 0 {
		opts.Faulty = true
	}
	hostConfig, err = oldconfig.LoadConfig("../test/data/exconf.json", opts)
	if err != nil {
		return err
	}

	for _, fh := range faultyNodes {
		fmt.Println("Setting", hostConfig.SNodes[fh].Name(), "as faulty")
		if failureRate == 100 {
			hostConfig.SNodes[fh].Host.(*coconet.FaultyHost).SetDeadFor("commit", true)

		}
		// hostConfig.SNodes[fh].Host.(*coconet.FaultyHost).Die()
	}

	if len(faultyNodes) > 0 {
		for i := range hostConfig.SNodes {
			hostConfig.SNodes[i].FailureRate = failureRate
		}
	}

	err = hostConfig.Run(signType)
	if err != nil {
		return err
	}
	// Have root node initiate the signing protocol via a simple annoucement
	hostConfig.SNodes[0].LogTest = []byte("Hello World")
	hostConfig.SNodes[0].Announce(&sign.AnnouncementMessage{LogTest: hostConfig.SNodes[0].LogTest, Round: 0})

	return nil
}

func TestTreeFromBigConfig(t *testing.T) {
	hc, err := oldconfig.LoadConfig("../test/data/exwax.json")
	if err != nil {
		t.Fatal()
	}
	err = hc.Run(sign.MerkleTree)
	if err != nil {
		t.Fatal(err)
	}
	// give it some time to set up
	time.Sleep(2 * time.Second)

	hc.SNodes[0].LogTest = []byte("hello world")
	err = hc.SNodes[0].Announce(&sign.AnnouncementMessage{LogTest: hc.SNodes[0].LogTest, Round: 0})
	if err != nil {
		t.Error(err)
	}
}

// tree from configuration file data/exconf.json
func TestMultipleRounds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	hostConfig, err := oldconfig.LoadConfig("../test/data/exconf.json")
	if err != nil {
		t.Fatal(err)
	}
	N := 5
	err = hostConfig.Run(sign.MerkleTree)
	if err != nil {
		t.Fatal(err)
	}
	// give it some time to set up
	time.Sleep(2 * time.Second)

	// Have root node initiate the signing protocol
	// via a simple annoucement
	for i := 0; i < N; i++ {
		hostConfig.SNodes[0].LogTest = []byte("Hello World" + strconv.Itoa(i))
		err = hostConfig.SNodes[0].Announce(&sign.AnnouncementMessage{LogTest: hostConfig.SNodes[0].LogTest, Round: i})
		if err != nil {
			t.Error(err)
		}
	}
}

func TestTCPStaticConfig(t *testing.T) {
	hc, err := oldconfig.LoadConfig("../test/data/extcpconf.json", oldconfig.ConfigOptions{ConnType: "tcp", GenHosts: true})
	if err != nil {
		t.Error(err)
	}
	err = hc.Run(sign.MerkleTree)
	if err != nil {
		t.Fatal(err)
	}
	// give it some time to set up
	time.Sleep(2 * time.Second)

	hc.SNodes[0].LogTest = []byte("hello world")
	err = hc.SNodes[0].Announce(&sign.AnnouncementMessage{LogTest: hc.SNodes[0].LogTest, Round: 0})
	if err != nil {
		t.Error(err)
	}
	for _, n := range hc.SNodes {
		n.Close()
	}
	log.Println("Test Done")
}

func TestTCPStaticConfigRounds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	hc, err := oldconfig.LoadConfig("../test/data/extcpconf.json", oldconfig.ConfigOptions{ConnType: "tcp", GenHosts: true})
	if err != nil {
		t.Error(err)
	}
	err = hc.Run(sign.MerkleTree)
	if err != nil {
		t.Fatal(err)
	}
	// give it some time to set up
	time.Sleep(2 * time.Second)

	N := 5
	for i := 0; i < N; i++ {
		hc.SNodes[0].LogTest = []byte("hello world")
		err = hc.SNodes[0].Announce(&sign.AnnouncementMessage{LogTest: hc.SNodes[0].LogTest, Round: i})

		if err != nil {
			t.Error(err)
		}
	}
	time.Sleep(10 * time.Second)
	for _, n := range hc.SNodes {
		n.Close()
	}
}

// func TestTreeBigConfigTCP(t *testing.T) {
// 	if testing.Short() {
// 		t.Skip("skipping test in short mode.")
// 	}
// 	hc, err := LoadConfig("data/wax.json", ConfigOptions{ConnType: "tcp", GenHosts: true})
// 	if err != nil {
// 		t.Error()
// 	}
// 	err = hc.Run(sign.MerkleTree)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	hc.SNodes[0].LogTest = []byte("hello world")
// 	err = hc.SNodes[0].Announce(&AnnouncementMessage{hc.SNodes[0].LogTest})
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	for _, n := range hc.SNodes {
// 		n.Close()
// 	}
// }

/*func BenchmarkTreeBigConfigTCP(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping test in short mode.")
	}
	hc, err := LoadConfig("data/wax.json", "tcp")
	if err != nil {
		b.Error()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hc.SNodes[0].LogTest = []byte("hello world")
		hc.SNodes[0].Announce(&AnnouncementMessage{hc.SNodes[0].LogTest})
	}
}*/
