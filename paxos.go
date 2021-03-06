package paxos

//
// Paxos library, to be included in an application.
// Multiple applications will run, each including
// a Paxos peer.
//
// Manages a sequence of agreed-on values.
// The set of peers is fixed.
// Copes with network failures (partition, msg loss, &c).
// Does not store anything persistently, so cannot handle crash+restart.
//
// The application interface:
//
// px = paxos.Make(peers []string, me string)
// px.Start(seq int, v interface{}) -- start agreement on new instance
// px.Status(seq int) (Fate, v interface{}) -- get info about an instance
// px.Done(seq int) -- ok to forget all instances <= seq
// px.Max() int -- highest instance seq known, or -1
// px.Min() int -- instances before this seq have been forgotten
//

import "net"
import "net/rpc"
import "log"

import "os"
import "syscall"
import "sync"
import "sync/atomic"
import "fmt"
import (
	"math/rand"
	"strconv"
	"time"
)

// new struct

const (
	OK = "OK"
	Reject = "Reject"
)

type PrepareArgs struct {
	Seq int		//the instance id
	PNum string	//the epoch number
}

type PrepareReply struct {
	Err string
	AcceptPnum string
	AcceptValue interface {}
}

type AcceptArgs struct {
	Seq int
	PNum string
	Value interface {}
}

type AcceptReply struct  {
	Err string
}

type DecideArgs struct {
	Seq int
	Value interface {}
	PNum string
	Me int
	Done int
}

type DecideReply struct {

}

// helper functions
func (px *Paxos) newInstance() *instance {
	return &instance{n_a: "", n_p: "", v_a: nil, state: Pending}
}

func (px *Paxos) majority() int {
	return len(px.peers)/2 + 1
}

// generate a proposer num
func (px *Paxos) generatePNum() string {
	begin := time.Date(2017, time.April, 4, 19, 0, 0, 0, time.UTC)
	duration := time.Now().Sub(begin)
	return strconv.FormatInt(duration.Nanoseconds(), 10) + "-" + strconv.Itoa(px.me)
}


// px.Status() return values, indicating
// whether an agreement has been decided,
// or Paxos has not yet reached agreement,
// or it was agreed but forgotten (i.e. < Min()).
type Fate int

const (
	Decided   Fate = iota + 1
	Pending        // not yet decided.
	Forgotten      // decided but forgotten.
)

type instance struct {
	state Fate        // instance state
	n_p   string      // proposed epoch num
	n_a   string      // accepted epoch num
	v_a   interface{} // accepted value
}

type Paxos struct {
	mu         sync.Mutex
	l          net.Listener
	dead       int32 // for testing
	unreliable int32 // for testing
	rpcCount   int32 // for testing
	peers      []string // peers, index as id, str as ports
	me         int // index into peers[]

	// Your data here.
	dones []int	// the state of each peer
	instances	map[int]*instance // save the <Seq, instance> pair
}

//
// call() sends an RPC to the rpcname handler on server srv
// with arguments args, waits for the reply, and leaves the
// reply in reply. the reply argument should be a pointer
// to a reply structure.
//
// the return value is true if the server responded, and false
// if call() was not able to contact the server. in particular,
// the replys contents are only valid if call() returned true.
//
// you should assume that call() will time out and return an
// error after a while if it does not get a reply from the server.
//
// please use call() to send all RPCs, in client.go and server.go.
// please do not change this function.
//
func call(srv string, name string, args interface{}, reply interface{}) bool {
	c, err := rpc.Dial("unix", srv)
	if err != nil {
		err1 := err.(*net.OpError)
		if err1.Err != syscall.ENOENT && err1.Err != syscall.ECONNREFUSED {
			fmt.Printf("paxos Dial() failed: %v\n", err1)
		}
		return false
	}
	defer c.Close()

	err = c.Call(name, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}


// LabLabLab
func (px *Paxos) Prepare(args *PrepareArgs, reply *PrepareReply) error {
	// Your code here
	//first add the lock
	px.mu.Lock()
	defer px.mu.Unlock();
	//then check the Seq
	//maxseq := px.Max()
	_,ok := px.instances[args.Seq]
	if !ok {
		px.instances[args.Seq]=px.newInstance()
	}	
	maxseq := px.instances[args.Seq].n_p
	//set the reply
	//如果提议号大于接受者最大提议号，或目前无最大提议号，更新提议值和提议号
	if (args.PNum >= maxseq ) {
		reply.Err = OK
		px.instances[args.Seq].n_p = args.PNum
	}else{//如果提议号小于目前最大提议号,拒绝
		reply.Err = Reject
		//reply.AcceptPnum = maxseq
	}
	reply.AcceptValue = px.instances[args.Seq].v_a
	reply.AcceptPnum = px.instances[args.Seq].n_a
	return nil
}

// LabLabLab
func (px *Paxos) Accept(args *AcceptArgs, reply *AcceptReply) error {
	// Your code here
	// first add the lock
	px.mu.Lock()
	defer px.mu.Unlock()
	// then check the Seq
	
	_,ok := px.instances[args.Seq]
	//未prepare，拒绝
	if !ok {
		/*px.instances[args.Seq] = px.newInstance()
		px.instances[args.Seq].n_p = args.PNum
		px.instances[args.Seq].n_a = args.PNum
		px.instances[args.Seq].v_a = args.Value*/
		reply.Err = Reject
	}else{
		maxseq := px.instances[args.Seq].n_p
		//以前提议号小于等于当前提议号，更新提议号和提议值
		if(args.PNum >= maxseq){
			reply.Err = OK
			px.instances[args.Seq].n_p = args.PNum
			px.instances[args.Seq].n_a = args.PNum
			px.instances[args.Seq].v_a = args.Value
			//px.instances[args.Seq].state = Decided
			//px.dones[args.Me] = args.Done

		}else{
			reply.Err = Reject
		}
	}

	
	
	// set the reply
	
	return nil
}

//accept the decided value from others
func (px *Paxos) Decide(args *DecideArgs, reply *DecideReply) error {
	// Your code here
	// first add the lock
	px.mu.Lock()
	defer px.mu.Unlock()
	//fmt.Println("Decide: %d, %d, %s", px.me, args.Seq, args.PNum)

	//then new the instance if not exist
	_, exist := px.instances[args.Seq]
	if !exist {
		px.instances[args.Seq] = px.newInstance()
	}

	//update the num and value
    // update proposer number,accept num and value,state
	px.instances[args.Seq].v_a = args.Value
	px.instances[args.Seq].n_a = args.PNum
	px.instances[args.Seq].n_p = args.PNum
	px.instances[args.Seq].state = Decided
    // update the server done array
	px.dones[args.Me] = args.Done
	return nil
}


func (px *Paxos) sendAccept(seq int, pnum string, v interface{}) bool {
	acargs := AcceptArgs{seq,pnum,v}
	accNum := 0
	for i,peer := range px.peers{
		acreply := AcceptReply{}

		if(i == px.me){
			px.Accept(&acargs,&acreply)
		}else{
			call(peer, "Paxos.Accept", &acargs, &acreply)

		}
		if(acreply.Err == OK){
			accNum+=1
		}
	}
    // return if qurom accept
	return accNum >= px.majority()
}




// LabLabLab
func (px *Paxos) propose(seq int, v interface{}) {
	// Your code here
	//fmt.Println("%d, try to propose: %d", px.me, seq)
	for {
		

		pnum := px.generatePNum()
		prepareargs := PrepareArgs{seq,pnum}
			
		acnum := 0
		maxprenum := ""
		maxacval := v
		for i, peer := range px.peers{
			preparereply := PrepareReply{AcceptValue: nil, AcceptPnum: "", Err: Reject}
			if(i == px.me){
				px.Prepare(&prepareargs,&preparereply)

			}else{
				call(peer, "Paxos.Prepare", &prepareargs, &preparereply)
			}
			if(preparereply.Err == OK){
				acnum +=1
				if(preparereply.AcceptPnum > maxprenum){
					maxprenum = preparereply.AcceptPnum
					maxacval = preparereply.AcceptValue
				}
			}
		}

		ok := false
		value := maxacval
		//超过半数prepare的OK回应
		if(acnum >= px.majority()){
			ok = true
		}
		//ok, pnum, value := px.sendPrepare(seq, v)
		
		if ok {
			ok = px.sendAccept(seq, pnum, value)
		}

		if(ok){
			decargs := DecideArgs{Seq: seq, Value: value, PNum: pnum, //maxacval
				Me: px.me, Done: px.dones[px.me]}
			for i, peer := range px.peers {
				var decreply DecideReply
				//fmt.Println("sendDecide: %d, %d, %s", px.me, decargs.Seq, decargs.PNum)
				if i == px.me {
					px.Decide(&decargs, &decreply)

				} else {
					call(peer, "Paxos.Decide", &decargs, &decreply)
				}
			}
			break
		}


		//tell other peers the dicided value, if majority agree
		/*if accNum >= px.majority() {

			decargs := DecideArgs{Seq: seq, Value: maxacval, PNum: pnum, 
				Me: px.me, Done: px.dones[px.me]}
			for i, peer := range px.peers {
				var decreply DecideReply
				//fmt.Println("sendDecide: %d, %d, %s", px.me, decargs.Seq, decargs.PNum)
				if i == px.me {
					px.Decide(&decargs, &decreply)

				} else {
					call(peer, "Paxos.Decide", &decargs, &decreply)
				}
			}
			break
		}*/

		state, _ := px.Status(seq)
		if state == Decided {
			break
		}
	}
}






//
// the application wants paxos to start agreement on
// instance seq, with proposed value v.
// Start() returns right away; the application will
// call Status() to find out if/when agreement
// is reached.
//
func (px *Paxos) Start(seq int, v interface{}) {
	// Your code here.
	//try to propose
	if seq < px.Min() {
		return
	}
	go func() {
		px.propose(seq, v)
	} ()
}

//
// the application on this machine is done with
// all instances <= seq.
//
// see the comments for Min() for more explanation.
//
func (px *Paxos) Done(seq int) {
	// Your code here.
	px.mu.Lock()
	defer px.mu.Unlock()

	if seq > px.dones[px.me] {
		px.dones[px.me] = seq
	}
}

//
// the application wants to know the
// highest instance sequence known to
// this peer.
//
func (px *Paxos) Max() int {
	// Your code here.
	max := 0
	for i, _ := range px.instances {
		if i > max {
			max = i
		}
	}
	return max
}

//
// Min() should return one more than the minimum among z_i,
// where z_i is the highest number ever passed
// to Done() on peer i. A peers z_i is -1 if it has
// never called Done().
//
// Paxos is required to have forgotten all information
// about any instances it knows that are < Min().
// The point is to free up memory in long-running
// Paxos-based servers.
//
// Paxos peers need to exchange their highest Done()
// arguments in order to implement Min(). These
// exchanges can be piggybacked on ordinary Paxos
// agreement protocol messages, so it is OK if one
// peers Min does not reflect another Peers Done()
// until after the next instance is agreed to.
//
// The fact that Min() is defined as a minimum over
// *all* Paxos peers means that Min() cannot increase until
// all peers have been heard from. So if a peer is dead
// or unreachable, other peers Min()s will not increase
// even if all reachable peers call Done. The reason for
// this is that when the unreachable peer comes back to
// life, it will need to catch up on instances that it
// missed -- the other peers therefor cannot forget these
// instances.
//
func (px *Paxos) Min() int {
	// You code here.
	px.mu.Lock()
	defer px.mu.Unlock()

	min := px.dones[px.me]
	for _, i := range px.dones {
		if i < min {
			min = i
		}
	}

	for seq, instance := range px.instances {
		if seq <= min && instance.state == Decided {
			delete(px.instances, seq)
		}
	}

	return min+1
}

//
// the application wants to know whether this
// peer thinks an instance has been decided,
// and if so what the agreed value is. Status()
// should just inspect the local peer state;
// it should not contact other Paxos peers.
//
func (px *Paxos) Status(seq int) (Fate, interface{}) {
	// Your code here.
	if seq < px.Min() {
		return Forgotten, nil
	}
	instance, exist := px.instances[seq]
	if !exist {
		return Pending, nil
	} else {
		return instance.state, instance.v_a
	}
	return Pending, nil
}



//
// tell the peer to shut itself down.
// for testing.
// please do not change these two functions.
//
func (px *Paxos) Kill() {
	atomic.StoreInt32(&px.dead, 1)
	if px.l != nil {
		px.l.Close()
	}
}

//
// has this peer been asked to shut down?
//
func (px *Paxos) isdead() bool {
	return atomic.LoadInt32(&px.dead) != 0
}

// please do not change these two functions.
func (px *Paxos) setunreliable(what bool) {
	if what {
		atomic.StoreInt32(&px.unreliable, 1)
	} else {
		atomic.StoreInt32(&px.unreliable, 0)
	}
}

func (px *Paxos) isunreliable() bool {
	return atomic.LoadInt32(&px.unreliable) != 0
}

//
// the application wants to create a paxos peer.
// the ports of all the paxos peers (including this one)
// are in peers[]. this servers port is peers[me].
//
func Make(peers []string, me int, rpcs *rpc.Server) *Paxos {
	px := &Paxos{}
	px.peers = peers
	px.me = me


	// Your initialization code here.
	px.instances = map[int]*instance{}
	px.dones = make([]int, len(px.peers))
	for i := range px.peers {
		px.dones[i] = -1
	}

	if rpcs != nil {
		// caller will create socket &c
		rpcs.Register(px)
	} else {
		rpcs = rpc.NewServer()
		rpcs.Register(px)

		// prepare to receive connections from clients.
		// change "unix" to "tcp" to use over a network.
		os.Remove(peers[me]) // only needed for "unix"
		l, e := net.Listen("unix", peers[me])
		if e != nil {
			log.Fatal("listen error: ", e)
		}
		px.l = l

		// please do not change any of the following code,
		// or do anything to subvert it.

		// create a thread to accept RPC connections
		go func() {
			for px.isdead() == false {
				conn, err := px.l.Accept()
				if err == nil && px.isdead() == false {
					if px.isunreliable() && (rand.Int63()%1000) < 100 {
						// discard the request.
						conn.Close()
					} else if px.isunreliable() && (rand.Int63()%1000) < 200 {
						// process the request but force discard of reply.
						c1 := conn.(*net.UnixConn)
						f, _ := c1.File()
						err := syscall.Shutdown(int(f.Fd()), syscall.SHUT_WR)
						if err != nil {
							fmt.Printf("shutdown: %v\n", err)
						}
						atomic.AddInt32(&px.rpcCount, 1)
						go rpcs.ServeConn(conn)
					} else {
						atomic.AddInt32(&px.rpcCount, 1)
						go rpcs.ServeConn(conn)
					}
				} else if err == nil {
					conn.Close()
				}
				if err != nil && px.isdead() == false {
					fmt.Printf("Paxos(%v) accept: %v\n", me, err.Error())
				}
			}
		}()
	}


	return px
}
