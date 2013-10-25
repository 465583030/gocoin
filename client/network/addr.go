package network

import (
	"time"
	"sync"
	"bytes"
	"encoding/binary"
	"github.com/piotrnar/gocoin/qdb"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/common"
)


var (
	ExternalIp4 map[uint32]uint = make(map[uint32]uint)
	ExternalIpMutex sync.Mutex
)


func ExternalAddrLen() (res int) {
	ExternalIpMutex.Lock()
	res = len(ExternalIp4)
	ExternalIpMutex.Unlock()
	return
}


func BestExternalAddr() []byte {
	var best_ip uint32
	var best_cnt uint
	ExternalIpMutex.Lock()
	for ip, cnt := range ExternalIp4 {
		if cnt > best_cnt {
			best_cnt = cnt
			best_ip = ip
		}
	}
	ExternalIpMutex.Unlock()
	res := make([]byte, 26)
	binary.LittleEndian.PutUint64(res[0:8], common.Services)
	// leave ip6 filled with zeros, except for the last 2 bytes:
	res[18], res[19] = 0xff, 0xff
	binary.BigEndian.PutUint32(res[20:24], best_ip)
	binary.BigEndian.PutUint16(res[24:26], common.DefaultTcpPort)
	return res
}


func (c *OneConnection) SendAddr() {
	pers := GetBestPeers(MaxAddrsPerMessage, false)
	if len(pers)>0 {
		buf := new(bytes.Buffer)
		btc.WriteVlen(buf, uint32(len(pers)))
		for i := range pers {
			binary.Write(buf, binary.LittleEndian, pers[i].Time)
			buf.Write(pers[i].NetAddr.Bytes())
		}
		c.SendRawMsg("addr", buf.Bytes())
	}
}


func (c *OneConnection) SendOwnAddr() {
	if ExternalAddrLen()>0 {
		buf := new(bytes.Buffer)
		btc.WriteVlen(buf, 1)
		binary.Write(buf, binary.LittleEndian, uint32(time.Now().Unix()))
		buf.Write(BestExternalAddr())
		c.SendRawMsg("addr", buf.Bytes())
	}
}

// Parese network's "addr" message
func ParseAddr(pl []byte) {
	b := bytes.NewBuffer(pl)
	cnt, _ := btc.ReadVLen(b)
	for i := 0; i < int(cnt); i++ {
		var buf [30]byte
		n, e := b.Read(buf[:])
		if n!=len(buf) || e!=nil {
			common.CountSafe("AddrError")
			//println("ParseAddr:", n, e)
			break
		}
		a := NewPeer(buf[:])
		if !ValidIp4(a.Ip4[:]) {
			common.CountSafe("AddrInvalid")
		} else if time.Unix(int64(a.Time), 0).Before(time.Now().Add(time.Minute)) {
			if time.Now().Before(time.Unix(int64(a.Time), 0).Add(ExpirePeerAfter)) {
				k := qdb.KeyType(a.UniqID())
				v := PeerDB.Get(k)
				if v != nil {
					a.Banned = NewPeer(v[:]).Banned
				}
				PeerDB.Put(k, a.Bytes())
			} else {
				common.CountSafe("AddrStale")
			}
		} else {
			common.CountSafe("AddrInFuture")
		}
	}
}
