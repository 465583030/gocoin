package network

import (
	"fmt"
	"net"
	"time"
	"bytes"
	"math/rand"
	"sync/atomic"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)


func (c *OneConnection) SendPendingData() bool {
	if c.SendBufProd!=c.SendBufCons {
		bytes_to_send := c.SendBufProd-c.SendBufCons
		if bytes_to_send<0 {
			bytes_to_send += SendBufSize
		}
		if c.SendBufCons+bytes_to_send > SendBufSize {
			bytes_to_send = SendBufSize-c.SendBufCons
		}

		n, e := common.SockWrite(c.Conn, c.sendBuf[c.SendBufCons:c.SendBufCons+bytes_to_send])
		if n > 0 {
			c.Mutex.Lock()
			c.X.LastSent = time.Now()
			c.X.BytesSent += uint64(n)
			n += c.SendBufCons
			if n >= SendBufSize {
				c.SendBufCons = 0
			} else {
				c.SendBufCons = n
			}
			c.Mutex.Unlock()
		} else if time.Now().After(c.X.LastSent.Add(AnySendTimeout)) {
			common.CountSafe("PeerSendTimeout")
			c.Disconnect()
		} else if e != nil {
			if common.DebugLevel > 0 {
				println(c.PeerAddr.Ip(), "Connection Broken during send")
			}
			c.Disconnect()
		}
	}
	return c.SendBufProd!=c.SendBufCons
}


// Call this once a minute
func (c *OneConnection) Maintanence(now time.Time) {
	// Disconnect and ban useless peers (such that don't send invs)
	if c.X.InvsRecieved==0 && c.X.ConnectedAt.Add(NO_INV_TIMEOUT).Before(now) {
		c.DoS("PeerUseless")
		return
	}

	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	// Expire GetBlockInProgress after five minutes, if they are not in BlocksToGet
	MutexRcv.Lock()
	for k, v := range c.GetBlockInProgress {
		if _, ok := BlocksToGet[k]; ok {
			continue
		}
		if now.After(v.start.Add(5*time.Minute)) {
			delete(c.GetBlockInProgress, k)
			common.CountSafe("BlockInprogTimeout")
			c.counters["BlockTimeout"]++
			//println(c.ConnID, "GetBlockInProgress timeout")
			break
		}
	}
	MutexRcv.Unlock()

	// Expire BlocksReceived after two days
	if len(c.blocksreceived)>0 {
		var i int
		for i=0; i<len(c.blocksreceived); i++ {
			if c.blocksreceived[i].Add(common.BlockExpireEvery).After(now) {
				break
			}
			common.CountSafe("BlksRcvdExpired")
		}
		if i>0 {
			//println(c.ConnID, "expire", i, "block(s)")
			c.blocksreceived = c.blocksreceived[i:]
		}
	}
}


func (c *OneConnection) Tick() {
	defer time.Sleep(50*time.Millisecond)

	now := time.Now()

	// Check no-data timeout
	if c.X.LastDataGot.Add(NO_DATA_TIMEOUT).Before(now) {
		c.Disconnect()
		common.CountSafe("NetNodataTout")
		if common.DebugLevel>0 {
			println(c.PeerAddr.Ip(), "no data for", NO_DATA_TIMEOUT/time.Second, "seconds - disconnect")
		}
		return
	}

	if !c.X.VerackReceived {
		// If we have no ack, do nothing more.
		return
	}

	if now.After(c.nextMaintanence) {
		c.Maintanence(now)
		c.nextMaintanence = now.Add(MAINTANENCE_PERIOD)
	}

	// Ask node for new addresses...?
	if !c.X.OurGetAddrDone && peersdb.PeerDB.Count() < common.MaxPeersNeeded {
		common.CountSafe("AddrWanted")
		c.SendRawMsg("getaddr", nil)
		c.X.OurGetAddrDone = true
	}

	do_getheaders_now := c.X.AllHeadersReceived==0 && !c.X.GetHeadersInProgress

	if !do_getheaders_now && !c.X.GetHeadersInProgress && c.BlksInProgress()==0 {
		if now.After(c.nextHdrsTime) {
			//println(c.ConnID, "RetryHeaders", c.X.AllHeadersReceived)
			do_getheaders_now = true
			c.IncCnt("RetryHeaders", 1)
		}
	}

	MutexRcv.Lock()
	if len(BlocksToGet) > 0 && !c.X.GetBlocksDataNow && now.Sub(c.X.LastFetchTried) > time.Second {
		c.X.GetBlocksDataNow = true
	}
	MutexRcv.Unlock()

	c.CheckGetBlockData()

	if do_getheaders_now {
		//println("fetch new headers from", c.PeerAddr.Ip(), blocks_to_get, len(NetBlocks))
		c.sendGetHeaders()
	}

	// Need to send some invs...?
	c.SendInvs()

	if !c.X.GetHeadersInProgress && c.BlksInProgress()==0 {
		// Ping if we dont do anything
		c.TryPing()
	}
}

func DoNetwork(ad *peersdb.PeerAddr) {
	var e error
	conn := NewConnection(ad)
	Mutex_net.Lock()
	if _, ok := OpenCons[ad.UniqID()]; ok {
		if common.DebugLevel>0 {
			fmt.Println(ad.Ip(), "already connected")
		}
		common.CountSafe("ConnectingAgain")
		Mutex_net.Unlock()
		return
	}
	OpenCons[ad.UniqID()] = conn
	OutConsActive++
	Mutex_net.Unlock()
	go func() {
		conn.Conn, e = net.DialTimeout("tcp4", fmt.Sprintf("%d.%d.%d.%d:%d",
			ad.Ip4[0], ad.Ip4[1], ad.Ip4[2], ad.Ip4[3], ad.Port), TCPDialTimeout)
		if e == nil {
			conn.X.ConnectedAt = time.Now()
			if common.DebugLevel!=0 {
				println("Connected to", ad.Ip())
			}
			conn.Run()
		} else {
			if common.DebugLevel!=0 {
				println("Could not connect to", ad.Ip())
			}
			//println(e.Error())
		}
		Mutex_net.Lock()
		delete(OpenCons, ad.UniqID())
		OutConsActive--
		Mutex_net.Unlock()
		ad.Dead()
	}()
}


var (
	TCPServerStarted bool
	next_drop_peer time.Time
	next_clean_hammers time.Time
)


// TCP server
func tcp_server() {
	ad, e := net.ResolveTCPAddr("tcp4", fmt.Sprint("0.0.0.0:", common.DefaultTcpPort))
	if e != nil {
		println("ResolveTCPAddr", e.Error())
		return
	}

	lis, e := net.ListenTCP("tcp4", ad)
	if e != nil {
		println("ListenTCP", e.Error())
		return
	}
	defer lis.Close()

	//fmt.Println("TCP server started at", ad.String())

	for common.IsListenTCP() {
		common.CountSafe("NetServerLoops")
		Mutex_net.Lock()
		ica := InConsActive
		Mutex_net.Unlock()
		if ica < atomic.LoadUint32(&common.CFG.Net.MaxInCons) {
			lis.SetDeadline(time.Now().Add(time.Second))
			tc, e := lis.AcceptTCP()
			if e == nil {
				var terminate bool

				if common.DebugLevel>0 {
					fmt.Println("Incoming connection from", tc.RemoteAddr().String())
				}
				// set port to default, for incomming connections
				ad, e := peersdb.NewPeerFromString(tc.RemoteAddr().String(), true)
				if e == nil {
					// Hammering protection
					HammeringMutex.Lock()
					ti, ok := RecentlyDisconencted[ad.NetAddr.Ip4]
					HammeringMutex.Unlock()
					if ok && time.Now().Sub(ti) < HammeringMinReconnect {
						//println(ad.Ip(), "is hammering within", time.Now().Sub(ti).String())
						common.CountSafe("BanHammerIn")
						ad.Ban()
						terminate = true
					}

					if !terminate {
						// Incoming IP passed all the initial checks - talk to it
						conn := NewConnection(ad)
						conn.X.ConnectedAt = time.Now()
						conn.X.Incomming = true
						conn.Conn = tc
						Mutex_net.Lock()
						if _, ok := OpenCons[ad.UniqID()]; ok {
							//fmt.Println(ad.Ip(), "already connected")
							common.CountSafe("SameIpReconnect")
							Mutex_net.Unlock()
							terminate = true
						} else {
							OpenCons[ad.UniqID()] = conn
							InConsActive++
							Mutex_net.Unlock()
							go func () {
								conn.Run()
								Mutex_net.Lock()
								delete(OpenCons, ad.UniqID())
								InConsActive--
								Mutex_net.Unlock()
							}()
						}
					}
				} else {
					if common.DebugLevel>0 {
						println("NewPeerFromString:", e.Error())
					}
					common.CountSafe("InConnRefused")
					terminate = true
				}

				// had any error occured - close teh TCP connection
				if terminate {
					tc.Close()
				}
			}
		} else {
			time.Sleep(1e9)
		}
	}
	Mutex_net.Lock()
	for _, c := range OpenCons {
		if c.X.Incomming {
			c.Disconnect()
		}
	}
	TCPServerStarted = false
	Mutex_net.Unlock()
	//fmt.Println("TCP server stopped")
}


func NetworkTick() {
	if common.IsListenTCP() {
		if !TCPServerStarted {
			TCPServerStarted = true
			go tcp_server()
		}
	}

	Mutex_net.Lock()
	conn_cnt := OutConsActive
	Mutex_net.Unlock()

	if common.CFG.DropPeers.DropEachMinutes!=0 {
		if next_drop_peer.IsZero() {
			next_drop_peer = time.Now().Add(common.DropSlowestEvery)
		} else if time.Now().After(next_drop_peer) {
			drop_worst_peer()
			next_drop_peer = time.Now().Add(common.DropSlowestEvery)
		}
	}

	// hammering protection - expire recently disconnected
	if next_clean_hammers.IsZero() {
		next_clean_hammers = time.Now().Add(HammeringMinReconnect)
	} else if time.Now().After(next_clean_hammers) {
		HammeringMutex.Lock()
		for k, t := range RecentlyDisconencted {
			if time.Now().Sub(t) >= HammeringMinReconnect {
				delete(RecentlyDisconencted, k)
			}
		}
		HammeringMutex.Unlock()
		next_clean_hammers = time.Now().Add(HammeringMinReconnect)
	}

	for conn_cnt < atomic.LoadUint32(&common.CFG.Net.MaxOutCons) {
		adrs := peersdb.GetBestPeers(128, ConnectionActive)
		if len(adrs)==0 {
			common.LockCfg()
			if common.CFG.ConnectOnly=="" && common.DebugLevel>0 {
				println("no new peers", len(OpenCons), conn_cnt)
			}
			common.UnlockCfg()
			break
		}
		DoNetwork(adrs[rand.Int31n(int32(len(adrs)))])
		Mutex_net.Lock()
		conn_cnt = OutConsActive
		Mutex_net.Unlock()
	}
}


// Process that handles communication with a single peer
func (c *OneConnection) Run() {
	c.SendVersion()

	c.Mutex.Lock()
	now := time.Now()
	c.X.LastDataGot = now
	c.nextMaintanence = now.Add(time.Minute)
	c.LastPingSent = now.Add(5*time.Second-common.PingPeerEvery) // do first ping ~5 seconds from now
	c.X.AllHeadersReceived = 0
	c.Mutex.Unlock()


	for !c.IsBroken() {
		if c.IsBroken() {
			break
		}

		if c.SendPendingData() {
			continue // Do now read the socket if we have pending data to send
		}

		cmd := c.FetchMessage()

		if cmd==nil {
			c.Tick()
			continue
		}

		if c.X.VerackReceived {
			c.PeerAddr.Alive()
		}
		c.Mutex.Lock()
		c.counters["rcvd_"+cmd.cmd]++
		c.counters["rbts_"+cmd.cmd] += uint64(len(cmd.pl))
		c.X.LastDataGot = time.Now()
		c.X.LastCmdRcvd = cmd.cmd
		c.X.LastBtsRcvd = uint32(len(cmd.pl))
		c.Mutex.Unlock()

		if common.DebugLevel<0 {
			fmt.Println(c.PeerAddr.Ip(), "->", cmd.cmd, len(cmd.pl))
		}

		common.CountSafe("rcvd_"+cmd.cmd)
		common.CountSafeAdd("rbts_"+cmd.cmd, uint64(len(cmd.pl)))

		switch cmd.cmd {
			case "version":
				er := c.HandleVersion(cmd.pl)
				if er != nil {
					println("version msg error:", er.Error())
					c.Disconnect()
					break
				}
				if c.Node.DoNotRelayTxs {
					c.DoS("SPV")
					break
				}
				if c.Node.Version >= 70012 {
					c.SendRawMsg("sendheaders", nil)
					if c.Node.Version >= 70013 {
						if common.CFG.TXPool.FeePerByte!=0 {
							var pl [8]byte
							binary.LittleEndian.PutUint64(pl[:], 1000*common.CFG.TXPool.FeePerByte)
							c.SendRawMsg("feefilter", pl[:])
						}
						if c.Node.Version >= 70014 {
							if (c.Node.Services&SERVICE_SEGWIT)==0 {
								c.SendRawMsg("sendcmpct", []byte{0x01,0x01,0x00,0x00,0x00,0x00,0x00,0x00,0x00})
							} else {
								c.SendRawMsg("sendcmpct", []byte{0x01,0x02,0x00,0x00,0x00,0x00,0x00,0x00,0x00})
							}
						}
					}
				}

			case "verack":
				c.X.VerackReceived = true
				if common.IsListenTCP() {
					c.SendOwnAddr()
				}

			case "inv":
				c.ProcessInv(cmd.pl)
				c.CheckGetBlockData()

			case "tx":
				if common.CFG.TXPool.Enabled {
					c.ParseTxNet(cmd.pl)
				}

			case "addr":
				c.ParseAddr(cmd.pl)

			case "block": //block received
				netBlockReceived(c, cmd.pl)

			case "getblocks":
				c.GetBlocks(cmd.pl)

			case "getdata":
				c.ProcessGetData(cmd.pl)

			case "getaddr":
				if !c.X.GetAddrDone {
					c.SendAddr()
					c.X.GetAddrDone = true
				} else {
					c.Mutex.Lock()
					c.counters["SecondGetAddr"]++
					c.Mutex.Unlock()
					if c.Misbehave("SecondGetAddr", 1000/20) {
						break
					}
				}

			case "ping":
				re := make([]byte, len(cmd.pl))
				copy(re, cmd.pl)
				c.SendRawMsg("pong", re)

			case "pong":
				if c.PingInProgress==nil {
					common.CountSafe("PongUnexpected")
				} else if bytes.Equal(cmd.pl, c.PingInProgress) {
					c.HandlePong()
				} else {
					common.CountSafe("PongMismatch")
				}

			case "getheaders":
				c.GetHeaders(cmd.pl)

			case "notfound":
				common.CountSafe("NotFound")

			case "headers":
				c.HandleHeaders(cmd.pl)
				c.CheckGetBlockData()

			case "sendheaders":
				c.Node.SendHeaders = true

			case "feefilter":
				if len(cmd.pl) >= 8 {
					c.X.MinFeeSPKB = int64(binary.LittleEndian.Uint64(cmd.pl[:8]))
					//println(c.PeerAddr.Ip(), c.Node.Agent, "feefilter", c.X.MinFeeSPKB)
				}

			case "sendcmpct":
				if len(cmd.pl)>=9 {
					version := binary.LittleEndian.Uint64(cmd.pl[1:9])
					if version > c.Node.SendCmpctVer {
						//println(c.ConnID, "sendcmpct", cmd.pl[0])
						c.Node.SendCmpctVer = version
						c.Node.HighBandwidth = cmd.pl[0]==1
					} else {
						c.Mutex.Lock()
						c.counters[fmt.Sprint("SendCmpctV", version)]++
						c.Mutex.Unlock()
					}
				} else {
					println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "sendcmpct", hex.EncodeToString(cmd.pl))
				}

			case "cmpctblock":
				c.ProcessCmpctBlock(cmd.pl)

			case "getblocktxn":
				c.ProcessGetBlockTxn(cmd.pl)
				//println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "getblocktxn", hex.EncodeToString(cmd.pl))

			case "blocktxn":
				c.ProcessBlockTxn(cmd.pl)
				//println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn", hex.EncodeToString(cmd.pl))

			default:
				if common.DebugLevel>0 {
					println(cmd.cmd, "from", c.PeerAddr.Ip())
				}
		}
	}


	c.Mutex.Lock()
	MutexRcv.Lock()
	for k, _ := range c.GetBlockInProgress {
		if rec, ok := BlocksToGet[k] ; ok {
			rec.InProgress--
		} else {
			//println("ERROR! Block", bip.hash.String(), "in progress, but not in BlocksToGet")
		}
	}
	MutexRcv.Unlock()

	ban := c.banit
	c.Mutex.Unlock()
	if ban {
		c.PeerAddr.Ban()
		common.CountSafe("PeersBanned")
	} else if c.X.Incomming {
		HammeringMutex.Lock()
		RecentlyDisconencted[c.PeerAddr.NetAddr.Ip4] = time.Now()
		HammeringMutex.Unlock()
	}
	if common.DebugLevel!=0 {
		println("Disconnected from", c.PeerAddr.Ip())
	}
	c.Conn.Close()
}
