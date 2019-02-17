// A package clause starts every source file.
package main

import (
	"fmt"
	"crypto/sha256"
	"crypto/rsa"
	"crypto/rand"
	"math/big"
	"strconv"
	"strings"
	// "reflect"
	"sync" // for locks
	"github.com/streadway/amqp" // for rabbitmq
	log "github.com/sirupsen/logrus" // for logging
	"os"
)

// ELASTICO_STATES - states reperesenting the running state of the node
var ELASTICO_STATES = map[string]int{"NONE": 0, "PoW Computed": 1, "Formed Identity" : 2, "Formed Committee": 3, "RunAsDirectory": 4 ,"RunAsDirectory after-TxnReceived" : 5,  "RunAsDirectory after-TxnMulticast" : 6, "Receiving Committee Members" : 7,"PBFT_NONE" : 8 , "PBFT_PRE_PREPARE" : 9, "PBFT_PRE_PREPARE_SENT"  : 10, "PBFT_PREPARE_SENT" : 11, "PBFT_PREPARED" : 12, "PBFT_COMMITTED" : 13, "PBFT_COMMIT_SENT" : 14,  "Intra Consensus Result Sent to Final" : 15,  "Merged Consensus Data" : 16, "FinalPBFT_NONE" : 17,  "FinalPBFT_PRE_PREPARE" : 18, "FinalPBFT_PRE_PREPARE_SENT"  : 19,  "FinalPBFT_PREPARE_SENT" : 20 , "FinalPBFT_PREPARED" : 21, "FinalPBFT_COMMIT_SENT" : 22, "FinalPBFT_COMMITTED" : 23, "PBFT Finished-FinalCommittee" : 24 , "CommitmentSentToFinal" : 25, "FinalBlockSent" : 26, "FinalBlockReceived" : 27,"BroadcastedR" : 28, "ReceivedR" :  29, "FinalBlockSentToClient" : 30,   "LedgerUpdated" : 31}

// shared lock among processes
var lock sync.Mutex
// shared port among the processes 
var port int = 49152

// n : number of nodes
var n int = 66 
// s - where 2^s is the number of committees
var s int = 2
// c - size of committee
var c int = 4
// D - difficulty level , leading bits of PoW must have D 0's (keep w.r.t to hex)
var D int = 6
// r - number of bits in random string
var r int64 = 4
// fin_num - final committee id
var fin_num int = 0

func failOnError(err error, msg string) {
	// logging the error
	if err != nil {
		log.Error("see the error!")
		log.Error("%s: %s", msg, err)
		os.Exit()
	}
}


func random_gen(r int64) (*big.Int) {
	/*
		generate a random integer
	*/
	// n is the base, e is the exponent, creating big.Int variables
	var n,e = big.NewInt(2) , big.NewInt(r)
	// taking the exponent n to the power e and nil modulo, and storing the result in n
	n.Exp(n, e, nil)
	// generates the random num in the range[0,n)
	// here Reader is a global, shared instance of a cryptographically secure random number generator.
	randomNum, err := rand.Int(rand.Reader, n)

	if err != nil {
		fmt.Println("error:", err.Error)
	}
	return randomNum
}

// structure for identity of nodes
type Identity struct{
	IP string
	PK *rsa.PublicKey
	committee_id int64
	PoW map[string]interface{}
	epoch_randomness string
	port int
}

func (i *Identity) IdentityInit(){
	i.PoW = make(map[string]interface{})
}


func (i *Identity)isEqual(identityobj *Identity) bool{
	/*
		checking two objects of Identity class are equal or not
		
	*/
	return i.IP == identityobj.IP && i.PK == identityobj.PK && i.committee_id == identityobj.committee_id && i.PoW["hash"] == identityobj.PoW["hash"] && i.PoW["set_of_Rs"] == identityobj.PoW["set_of_Rs"] && i.PoW["nonce"] == identityobj.PoW["nonce"] &&i.epoch_randomness == identityobj.epoch_randomness && i.port == identityobj.port
}


func(i *Identity)send(msg map[string]interface{}){
	/*
		send the msg to node based on their identity
	*/
	// establish a connection with RabbitMQ server
	connection , err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	// report the error
	failOnError(err, "Failed to connect to RabbitMQ")
	// close the connection
	defer connection.Close()
	// create a channel
	ch, er := connection.Channel()
	// report the error
	failOnError(er, "Failed to open a channel")
	// close the channel
	defer ch.Close()
	port := strconv.Itoa(i.port)

	//create a hello queue to which the message will be delivered
	queue, err := ch.QueueDeclare(
		"hello" + port,	//name of the queue
		false,	// durable
		false,	// delete when unused
		false,	// exclusive
		false,	// no-wait
		nil,	// arguments
	)
	failOnError(err, "Failed to declare a queue")

	body := msg
	err = ch.Publish(
		"",				// exchange
		queue.Name,		// routing key
		false,			// mandatory
		false,			// immediate
		amqp.Publishing {
		ContentType: "text/plain",
		Body:		[]byte(msg),
	})

	failOnError(err, "Failed to publish a message")
}


type Transaction struct{
	sender string
	receiver string
	amount *big.Int // random_gen returns *big.Int
}

type Elastico struct{

	connection *amqp.Connection
	IP string
	port int
	key *rsa.PrivateKey
	PoW map[string]interface{}
	cur_directory []Identity
	identity Identity
	committee_id int64
	// only when this node is the member of directory committee
	committee_list map[int][]Identity
	// only when this node is not the member of directory committee
	committee_Members []Identity
	is_directory bool
	is_final bool
	epoch_randomness string
	Ri string
	// only when this node is the member of final committee
	commitments map[string]bool
	txn_block []Transaction
	set_of_Rs map[string]bool
	newset_of_Rs map[string]bool
	CommitteeConsensusData map[int]map[string][]string
	CommitteeConsensusDataTxns map[int]map[string][]Transaction
	finalBlockbyFinalCommittee map[int]map[string][]string
	finalBlockbyFinalCommitteeTxns map[int]map[string][]Transaction
	state int
	mergedBlock []Transaction
	finalBlock map[string]interface{}
	RcommitmentSet map[string]bool
	newRcommitmentSet map[string]bool
	finalCommitteeMembers []Identity
	// only when this is the member of the directory committee
	txn map[int][]Transaction
	response []Transaction
	flag bool
	views map[int]bool
	primary bool
	viewId int
	faulty bool
	 // pre_prepareMsgLog
	// prepareMsgLog
	// commitMsgLog
	// preparedData
	// committedData
	// Finalpre_prepareMsgLog
	// FinalprepareMsgLog
	// FinalcommitMsgLog
	// FinalpreparedData
	// FinalcommittedData
}


func (e *Elastico) get_key(){
	/*
		for each node, it will set key as public pvt key pair
	*/
	var err error
	// generate the public-pvt key pair
	e.key, err = rsa.GenerateKey(rand.Reader, 2048)
	failOnError(err, "key generation")
}


func (e *Elastico)get_IP(){
	/*
		for each node(processor) , get IP addr
	*/
	count := 4
	// construct the byte array of size 4
	byteArray := make([]byte, count)
	// Assigning random values to the byte array
	_, err := rand.Read(byteArray)
	failOnError(err, "reading random values error")
	// setting the IP addr from the byte array
	e.IP = fmt.Sprintf("%v.%v.%v.%v" , byteArray[0] , byteArray[1], byteArray[2], byteArray[3])
}


func (e *Elastico) initER(){
		/*
			initialise r-bit epoch random string
		*/

		randomnum := random_gen(r)
		// set r-bit binary string to epoch randomness
		e.epoch_randomness = fmt.Sprintf("%0"+ strconv.FormatInt(r, 10) + "b\n", randomnum)
}

func (e *Elastico)get_port(){
	/*
		get port number for the process
	*/
	// acquire the lock
	lock.Lock()
	port += 1
	e.port = port
	// release the lock
	defer lock.Unlock()
}


func (e* Elastico) compute_PoW(){
	/*	
		returns hash which satisfies the difficulty challenge(D) : PoW["hash"]
	*/
	zero_string := ""
	for i:=0 ; i < D; i ++ {
		zero_string += "0"
	}
	// ToDo: type assertion for interface
	nonce :=  e.PoW["nonce"].(int)
	if e.state == ELASTICO_STATES["NONE"] {
		// public key
		PK := e.key.Public()
		fmt.Printf("%T" , PK)
		rsaPublickey, err := PK.(*rsa.PublicKey)
		failOnError(err, "rsa Public key conversion")
		IP := e.IP
		// If it is the first epoch , randomset_R will be an empty set .
		// otherwise randomset_R will be any c/2 + 1 random strings Ri that node receives from the previous epoch
		randomset_R := make(map[string]bool)
		if len(e.set_of_Rs) > 0 {
			// ToDo: complete this for further epochs
			// e.epoch_randomness, randomset_R = e.xor_R()
		}
		// 	compute the digest 
		digest := sha256.New()
		digest.Write([]byte(IP))
		digest.Write(rsaPublickey.N.Bytes())
		digest.Write([]byte(strconv.Itoa(rsaPublickey.E)))
		digest.Write([]byte(e.epoch_randomness))
		digest.Write([]byte(strconv.Itoa(nonce)))

		hash_val := fmt.Sprintf("%x" , digest.Sum(nil))
		if strings.HasPrefix(hash_val, zero_string){
			//hash starts with leading D 0's
			e.PoW["hash"] = hash_val
			e.PoW["set_of_Rs"] =  randomset_R
			e.PoW["nonce"] = nonce
			// change the state after solving the puzzle
			e.state = ELASTICO_STATES["PoW Computed"]
		} else {
			// try for other nonce
			nonce += 1 
			e.PoW["nonce"] = nonce
		}
	}
}



func (e* Elastico) ElasticoInit() {
	var err error
	// create rabbit mq connection
	e.connection, err = amqp.Dial("amqp://guest:guest@localhost:5672/")
	// log if connection fails
	failOnError(err, "Failed to connect to RabbitMQ")
	// set IP
	e.get_IP()
	e.get_port()
	// set RSA
	e.get_key()
	// Initialize PoW!	
	e.PoW = make(map[string]interface{})
	e.PoW["hash"] = ""
	e.PoW["set_of_Rs"] = ""
	e.PoW["nonce"] = 0

	e.cur_directory = make([]Identity, 0)
	
	e.committee_list = make(map[int][]Identity)

	e.committee_Members = make([]Identity, 0)

	// for setting epoch_randomness
	e.initER()

	e.commitments = make(map[string]bool)

	e.txn_block = make([]Transaction, 0)

	e.set_of_Rs = make(map[string]bool)

	e.newset_of_Rs = make(map[string]bool)
	
	e.CommitteeConsensusData = make(map[int]map[string][]string)
	
	e.CommitteeConsensusDataTxns = make(map[int]map[string][]Transaction)
	
	e.finalBlockbyFinalCommittee = make(map[int]map[string][]string)

	e.finalBlockbyFinalCommitteeTxns = make(map[int]map[string][]Transaction)

	e.state = ELASTICO_STATES["NONE"]

	e.mergedBlock = make([]Transaction, 0)

	e.finalBlock = make(map[string]interface{})
	e.finalBlock["sent"] = false
	e.finalBlock["finalBlock"] = make([]Transaction,0)

	e.RcommitmentSet = make(map[string]bool)
	e.newRcommitmentSet = make(map[string]bool)
	e.finalCommitteeMembers = make([]Identity, 0)
	
	e.txn = make(map[int][]Transaction)
	e.response = make([]Transaction, 0)
	e.flag = true
	e.views = make(map[int]bool)
	e.primary = false
	e.viewId = 0
	e.faulty = false	
}


func (e *Elastico)get_committeeid(PoW string) int64{
	/*
		returns last s-bit of PoW["hash"] as Identity : committee_id
	*/ 
	bindigest := ""
	
	for i:=0 ; i < len(PoW); i++ {
		intVal, err := strconv.ParseInt( string(PoW[i]) ,16, 0) // converts hex string to integer
		failOnError(err , "string to int conversion error")
		bindigest += fmt.Sprintf("%04b", intVal) // converts intVal to 4 bit binary value
	}
	// take last s bits of the binary digest
	identity := bindigest[len(bindigest)-s:]
	iden, err := strconv.ParseInt(identity, 2 , 0) // converts binary string to integer
	failOnError(err , "binary to int conversion error")
	return iden
}


func (e *Elastico)executePoW{
	/*
		execute PoW
	*/
	if e.flag{
		// compute Pow for good node
		e.compute_PoW()
	} else{
		// compute Pow for bad node
		e.compute_fakePoW()
	}
}


func (e *Elastico) isFinalMember() bool{
	/*
		tell whether this node is a final committee member or not
	*/
	return e.is_final
}


func (e *Elastico) runPBFT(){
	/*
		Runs a Pbft instance for the intra-committee consensus
	*/
	if e.state == ELASTICO_STATES["PBFT_NONE"]{
		if e.primary{
			// construct pre-prepare msg
			pre_preparemsg := e.construct_pre_prepare()
			// multicasts the pre-prepare msg to replicas
			// ToDo: what if primary does not send the pre-prepare to one of the nodes
			e.send_pre_prepare(pre_preparemsg)

			// change the state of primary to pre-prepared 
			e.state = ELASTICO_STATES["PBFT_PRE_PREPARE_SENT"]
			// primary will log the pre-prepare msg for itself
			e.logPre_prepareMsg(pre_preparemsg)

		} else{

			// for non-primary members
			if e.is_pre_prepared(){
				e.state = ELASTICO_STATES["PBFT_PRE_PREPARE"]
			}
		}

	} else if e.state == ELASTICO_STATES["PBFT_PRE_PREPARE"]{

		if e.primary == false{
			
			// construct prepare msg
			// ToDo: verify whether the pre-prepare msg comes from various primaries or not
			preparemsgList := e.construct_prepare()
			// logging.warning("constructing prepares with port %s" , str(e.port))
			e.send_prepare(preparemsgList)
			e.state = ELASTICO_STATES["PBFT_PREPARE_SENT"]
		}

	} else if e.state ==ELASTICO_STATES["PBFT_PREPARE_SENT"] || e.state == ELASTICO_STATES["PBFT_PRE_PREPARE_SENT"]{
			// ToDo: if, primary has not changed its state to "PBFT_PREPARE_SENT"
			if e.isPrepared(){
				
				// logging.warning("prepared done by %s" , str(e.port))
				e.state = ELASTICO_STATES["PBFT_PREPARED"]

			} else if e.state == ELASTICO_STATES["PBFT_PREPARED"]{

				commitMsgList := e.construct_commit()
				e.send_commit(commitMsgList)
				e.state = ELASTICO_STATES["PBFT_COMMIT_SENT"]

			}else if e.state == ELASTICO_STATES["PBFT_COMMIT_SENT"]{
				
				if e.isCommitted(){
					
					// logging.warning("committed done by %s" , str(e.port))
					e.state = ELASTICO_STATES["PBFT_COMMITTED"]	
				}
			}
	}
}
			

func (e *Elastico) runFinalPBFT(){
	/*
		Run PBFT by final committee members
	*/	
		if self.state == ELASTICO_STATES["FinalPBFT_NONE"]{

			if e.primary{

				// construct pre-prepare msg
				finalpre_preparemsg := e.construct_Finalpre_prepare()
				// multicasts the pre-prepare msg to replicas
				e.send_pre_prepare(finalpre_preparemsg)

				// change the state of primary to pre-prepared 
				e.state = ELASTICO_STATES["FinalPBFT_PRE_PREPARE_SENT"]
				// primary will log the pre-prepare msg for itself
				e.logFinalPre_prepareMsg(finalpre_preparemsg)

			}else{

				// for non-primary members
				if e.is_Finalpre_prepared(){
					e.state = ELASTICO_STATES["FinalPBFT_PRE_PREPARE"]
				}
			}

		}else if e.state == ELASTICO_STATES["FinalPBFT_PRE_PREPARE"]{
			
			if e.primary == false{
				
				// construct prepare msg
				FinalpreparemsgList := e.construct_Finalprepare()
				e.send_prepare(FinalpreparemsgList)
				e.state = ELASTICO_STATES["FinalPBFT_PREPARE_SENT"]
			}
		} else if e.state ==ELASTICO_STATES["FinalPBFT_PREPARE_SENT"] || e.state == ELASTICO_STATES["FinalPBFT_PRE_PREPARE_SENT"]{

			// ToDo: primary has not changed its state to "FinalPBFT_PREPARE_SENT"
			if e.isFinalPrepared(){

				e.state = ELASTICO_STATES["FinalPBFT_PREPARED"]
			}
		}else if e.state == ELASTICO_STATES["FinalPBFT_PREPARED"]{

			commitMsgList := e.construct_Finalcommit()
			e.send_commit(commitMsgList)
			e.state = ELASTICO_STATES["FinalPBFT_COMMIT_SENT"]

		}else if e.state == ELASTICO_STATES["FinalPBFT_COMMIT_SENT"]{

			if e.isFinalCommitted(){

				// for viewId in e.FinalcommittedData:
				// 	for seqnum in e.FinalcommittedData[viewId]:
				// 		msgList = e.FinalcommittedData[viewId][seqnum]
				// 		for msg in msgList:
				// 			e.finalBlock["finalBlock"] = e.unionTxns(e.finalBlock["finalBlock"], msg)
				// finalTxnBlock = e.finalBlock["finalBlock"]
				// finalTxnBlock = list(finalTxnBlock)
				// # order them! Reason : to avoid errors in signatures as sets are unordered
				// # e.finalBlock["finalBlock"] = sorted(finalTxnBlock)
				// logging.warning("final block by port %s with final block %s" , str(e.port), str(e.finalBlock["finalBlock"]))
				e.state = ELASTICO_STATES["FinalPBFT_COMMITTED"]
			}
		}
}



func (e *Elastico) compute_fakePoW(){
	/*
		bad node generates the fake PoW
	*/
	// random fakeness 
	x := random_gen(32)
	_ , index := x.DivMod(x , big.NewInt(3) , big.NewInt(0))

	if index == 0{
		
		// Random hash with initial D hex digits 0s
		digest := sha256.New()
		ranHash := fmt.Sprintf("%x" , digest.Sum(nil))
		hash_val := ""
		for i:=0 ; i < D ; i++{
			hash_val += "0"
		}
		e.PoW["hash"] = hash_val + ranHash[D:]

	} else if index == 1{
		
		// computing an invalid PoW using less number of values in digest
		randomset_R := set()
		zero_string = ""
		for i:= 0 ; i <  D ; i++{
			zero_string += "0"
		}
		// if len(self.set_of_Rs) > 0:
		// 	self.epoch_randomness, randomset_R = self.xor_R()    
		for {

			digest := sha256.New()
			nonce:= e.PoW["nonce"]
			digest.Write([]byte(strconv.Itoa(nonce)))
			hash_val := fmt.Sprintf("%x" , digest.Sum(nil))
			if strings.HasPrefix(hash_val, zero_string){
				e.PoW["hash"] = hash_val
				e.PoW["set_of_Rs"] =  randomset_R
				e.PoW["nonce"] = nonce
			}else{
				// try for other nonce
				nonce += 1 
				e.PoW["nonce"] = nonce
			}
		}
	}
	
	else if index == 2{
		
		// computing a random PoW
		randomset_R := set()
		// if len(self.set_of_Rs) > 0:
		// 	self.epoch_randomness, randomset_R := self.xor_R()    
		digest := sha256.New()
		ranHash := fmt.Sprintf("%x" , digest.Sum(nil))
		nonce := random_gen() 
		e.PoW["hash"] = ranHash
		e.PoW["set_of_Rs"] =  randomset_R
		// ToDo: nonce has to be in int instead of big.Int
		e.PoW["nonce"] = nonce
	}

	log.Warn("computed fake POW " , index)
	e.state = ELASTICO_STATES["PoW Computed"]
	
}


func (e *Elastico)form_identity() {
	/*
		identity formation for a node
		identity consists of public key, ip, committee id, PoW, nonce, epoch randomness
	*/	
	if e.state == ELASTICO_STATES["PoW Computed"]{
		// export public key
		PK := e.key.Public().(*rsa.PublicKey)

		// set the committee id acc to PoW solution
		e.committee_id = e.get_committeeid(e.PoW["hash"].(string))

		e.identity = Identity{e.IP, PK, e.committee_id, e.PoW, e.epoch_randomness,e.port}
		// changed the state after identity formation
		e.state = ELASTICO_STATES["Formed Identity"]
	}
}


func (e *Elastico)verify_PoW(identityobj){
	/*
		verify the PoW of the node identityobj
	*/
		zero_string := ""
		for i:=0 ; i < D; i++{

			zero_string += "0"
		}

		PoW := identityobj.PoW

		// length of hash in hex
		if len(PoW["hash"]) != 64{
			return false
		}

		// Valid Hash has D leading '0's (in hex)
		if ! strings.HasPrefix(Pow["hash"] , zero_string){
			return false
		}

		// check Digest for set of Ri strings
		// for Ri in PoW["set_of_Rs"]:
		// 	digest = self.hexdigest(Ri)
		// 	if digest not in self.RcommitmentSet:
		// 		return false

		// reconstruct epoch randomness
		epoch_randomness := identityobj.epoch_randomness
		// if len(PoW["set_of_Rs"]) > 0:
		// 	xor_val = 0
		// 	for R in PoW["set_of_Rs"]:
		// 		xor_val = xor_val ^ int(R, 2)
		// 	epoch_randomness = ("{:0" + str(r) +  "b}").format(xor_val)

		// recompute PoW 
		PK := identityobj.PK
		IP := identityobj.IP
		nonce := PoW["nonce"].(int)

		digest := sha256.New()
		digest.Write([]byte(IP))
		digest.update(PK.encode())
		digest.update([]byte(epoch_randomness))
		digest.update(str(nonce).encode())
		hash_val = digest.hexdigest()
		if hash_val.startswith('0' * D) && hash_val == PoW["hash"]:
			// Found a valid Pow, If this doesn't match with PoW["hash"] then Doesnt verify!
			return true
		return false
}
		


func (e *Elastico) execute(epochTxn){
	/*
		executing the functions based on the running state
	*/	
	// # print the current state of node for debug purpose
	// 		print(self.identity ,  list(ELASTICO_STATES.keys())[ list(ELASTICO_STATES.values()).index(self.state)], "STATE of a committee member")

	// initial state of elastico node
	if e.state == ELASTICO_STATES["NONE"]{

		e.executePoW()

	} else if e.state == ELASTICO_STATES["PoW Computed"]{
		
		// form identity, when PoW computed
		e.form_identity()
	} else if e.state == ELASTICO_STATES["Formed Identity"]{
		
		// form committee, when formed identity
		e.form_committee()

	} else if e.is_directory && e.state == ELASTICO_STATES["RunAsDirectory"]{
		
		log.Info("The directory member :- " , e.port)
		e.receiveTxns(epochTxn)
		// directory member has received the txns for all committees 
		e.state  = ELASTICO_STATES["RunAsDirectory after-TxnReceived"]

	} else if e.state == ELASTICO_STATES["Receiving Committee Members"]{
		// when a node is part of some committee	
		if e.flag == False{
			
			// logging the bad nodes
			logging.error("member with invalid POW %s with commMembers : %s", e.identity , e.committee_Members)
		}
		// Now The node should go for Intra committee consensus
		// initial state for the PBFT
		e.state = ELASTICO_STATES["PBFT_NONE"]
		// run PBFT for intra-committee consensus
		e.runPBFT("intra committee consensus")

	} else if e.state == ELASTICO_STATES["PBFT_NONE"] || e.state == ELASTICO_STATES["PBFT_PRE_PREPARE"] || e.state ==ELASTICO_STATES["PBFT_PREPARE_SENT"] || e.state == ELASTICO_STATES["PBFT_PREPARED"] || e.state == ELASTICO_STATES["PBFT_COMMIT_SENT"] || e.state == ELASTICO_STATES["PBFT_PRE_PREPARE_SENT"]{
		
		// run pbft for intra consensus
		e.runPBFT("intra committee consensus")
	} else if e.state == ELASTICO_STATES["PBFT_COMMITTED"]{

		// send pbft consensus blocks to final committee members
		log.Info("pbft finished by members %s" , str(e.port))
		e.SendtoFinal()

	}else if e.isFinalMember() && e.state == ELASTICO_STATES["Intra Consensus Result Sent to Final"]{
		
		// final committee node will collect blocks and merge them
		e.checkCountForConsensusData()

	}else if e.isFinalMember() && e.state == ELASTICO_STATES["Merged Consensus Data"]{
		
		// final committee member runs final pbft
		e.state = ELASTICO_STATES["FinalPBFT_NONE"]
		e.runFinalPBFT("final committee consensus")
	}else if e.state == ELASTICO_STATES["FinalPBFT_NONE"] || e.state == ELASTICO_STATES["FinalPBFT_PRE_PREPARE"] || e.state ==ELASTICO_STATES["FinalPBFT_PREPARE_SENT"] || e.state == ELASTICO_STATES["FinalPBFT_PREPARED"] || e.state == ELASTICO_STATES["FinalPBFT_COMMIT_SENT"] || e.state == ELASTICO_STATES["FinalPBFT_PRE_PREPARE_SENT"]{

		e.runFinalPBFT("final committee consensus")
	} else if e.isFinalMember() && e.state == ELASTICO_STATES["FinalPBFT_COMMITTED"]{

		// send the commitment to other final committee members
		e.sendCommitment()
		log.Warn("pbft finished by final committee %s" , str(e.port))
	}

	else if e.isFinalMember() && e.state == ELASTICO_STATES["CommitmentSentToFinal"]{

		// broadcast final txn block to ntw
		if len(e.commitments) >= c / 2 + 1 {
			e.BroadcastFinalTxn()
		}
	} else if e.state == ELASTICO_STATES["FinalBlockReceived"]{

		e.checkCountForFinalData()

	} else if e.isFinalMember() && e.state == ELASTICO_STATES["FinalBlockSentToClient"]{

		// broadcast Ri is done when received commitment has atleast c/2  + 1 signatures
		if len(e.newRcommitmentSet) >= c/2 + 1{
			e.BroadcastR()
		}
	}else if e.state == ELASTICO_STATES["ReceivedR"]{

		e.appendToLedger()
		e.state = ELASTICO_STATES["LedgerUpdated"]

	}else if e.state == ELASTICO_STATES["LedgerUpdated"]{

		// Now, the node can be reset
		return "reset"
	}
}


func createTxns()[]Transaction{
	/*
		create txns for an epoch
	*/
	// number of transactions in each epoch
	numOfTxns := 20
	// txns is the list of the transactions in one epoch to which the committees will agree on
	txns := make([]Transaction,numOfTxns)
	for i:=0 ; i < numOfTxns ; i++{
		// random amount
		random_num := random_gen(32)
		// create the dummy transaction
		transaction := Transaction{"a" , "b" , random_num}
		txns[i] = transaction
	}
	return txns
	
}


func main(){
	os.Remove("logfile.log")
	file, _ := os.OpenFile("logfile.log",  os.O_CREATE|os.O_APPEND | os.O_WRONLY , 0666)

	numOfEpochs := 2
	epochTxns := make(map[int][]Transaction)
	for epoch := 0 ; epoch < numOfEpochs ; epoch ++{
		epochTxns[epoch] = createTxns()
	}

	log.SetOutput(file)
	log.SetLevel(log.InfoLevel)

	// run all the epochs 
	// Run(epochTxns)

}