package textui

import (
	"fmt"
	"sort"
	"strconv"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
)


type OneWalletAddrs struct {
	P2SH bool
	Key [20]byte
	rec *wallet.OneAllAddrBal
}

type SortedWalletAddrs []OneWalletAddrs

var sort_by_cnt bool

func (sk SortedWalletAddrs) Len() int {
	return len(sk)
}

func (sk SortedWalletAddrs) Less(a, b int) bool {
	if sort_by_cnt {
		return len(sk[a].rec.Unsp) > len(sk[b].rec.Unsp)
	}
	return sk[a].rec.Value > sk[b].rec.Value
}

func (sk SortedWalletAddrs) Swap(a, b int) {
	sk[a], sk[b] = sk[b], sk[a]
}


func max_outs(par string) {
	sort_by_cnt = true
	all_addrs(par)
}

func best_val(par string) {
	sort_by_cnt = false
	all_addrs(par)
}



func all_addrs(par string) {
	var tot_val, tot_inps, ptsh_outs, ptsh_vals uint64
	var best SortedWalletAddrs
	var cnt int = 15

	if par!="" {
		if c, e := strconv.ParseUint(par, 10, 32); e==nil {
			cnt = int(c)
		}
	}

	wallet.BalanceMutex.Lock()
	defer wallet.BalanceMutex.Unlock()

	for k, rec := range wallet.AllBalancesP2SH {
		tot_val += rec.Value
		tot_inps += uint64(len(rec.Unsp))
		if sort_by_cnt && len(rec.Unsp)>=1000 || !sort_by_cnt && rec.Value>=1000e8 {
			best = append(best, OneWalletAddrs{P2SH:true, Key:k, rec:rec})
		}
	}
	ptsh_outs = tot_val
	ptsh_vals = tot_inps

	for k, rec := range wallet.AllBalancesP2KH {
		tot_val += rec.Value
		tot_inps += uint64(len(rec.Unsp))
		if sort_by_cnt && len(rec.Unsp)>=1000 || !sort_by_cnt && rec.Value>=1000e8 {
			best = append(best, OneWalletAddrs{Key:k, rec:rec})
		}
	}
	fmt.Println(btc.UintToBtc(tot_val), "BTC in", tot_inps, "unspent records from", len(wallet.AllBalancesP2SH)+len(wallet.AllBalancesP2KH), "addresses")
	fmt.Println(btc.UintToBtc(ptsh_vals), "BTC in", ptsh_outs, "records from", len(wallet.AllBalancesP2SH), "P2SH addresses")

	if sort_by_cnt {
		fmt.Println("Addrs with at least 1000 inps:", len(best))
	} else {
		fmt.Println("Addrs with at least 1000 BTC:", len(best))
	}

	sort.Sort(best)

	var pkscr_p2sk [23]byte
	var pkscr_p2kh [25]byte
	var ad *btc.BtcAddr

	pkscr_p2sk[0] = 0xa9
	pkscr_p2sk[1] = 20
	pkscr_p2sk[22] = 0x87

	pkscr_p2kh[0] = 0x76
	pkscr_p2kh[1] = 0xa9
	pkscr_p2kh[2] = 20
	pkscr_p2kh[23] = 0x88
	pkscr_p2kh[24] = 0xac

	for i:=0; i<len(best) && i<cnt; i++ {
		if best[i].P2SH {
			copy(pkscr_p2sk[2:22], best[i].Key[:])
			ad = btc.NewAddrFromPkScript(pkscr_p2sk[:], common.CFG.Testnet)
		} else {
			copy(pkscr_p2kh[3:23], best[i].Key[:])
			ad = btc.NewAddrFromPkScript(pkscr_p2kh[:], common.CFG.Testnet)
		}
		fmt.Println(i+1, ad.String(), btc.UintToBtc(best[i].rec.Value), "BTC in", len(best[i].rec.Unsp), "inputs")
	}
}

func list_unspent(addr string) {
	fmt.Println("Checking unspent coins for addr", addr)

	ad, e := btc.NewAddrFromString(addr)
	if e != nil {
		println(e.Error())
		return
	}

	wallet.BalanceMutex.Lock()
	defer wallet.BalanceMutex.Unlock()

	var rec *wallet.OneAllAddrBal

	if ad.Version==btc.AddrVerPubkey(common.Testnet) {
		rec = wallet.AllBalancesP2KH[ad.Hash160]
	} else if ad.Version==btc.AddrVerScript(common.Testnet) {
		rec = wallet.AllBalancesP2SH[ad.Hash160]
	} else {
		fmt.Println("Only P2SH and P2KH address types are supported")
		return
	}

	if rec == nil {
		fmt.Println(ad.String(), "has no coins")
	} else {
		fmt.Println(ad.String(), "has", btc.UintToBtc(rec.Value&0x7fffffffffff), "BTC in", len(rec.Unsp), "records")
		for i := range rec.Unsp {
			uns, vo := rec.Unsp[i].GetRec()
			fmt.Println("", i+1, fmt.Sprint(btc.NewUint256(uns.TxID[:]).String(), "-", vo),
				"from block", uns.InBlock, uns.Coinbase, btc.UintToBtc(uns.Outs[vo].Value), "BTC")
		}
	}
}

func init() {
	newUi("richest r", true, best_val, "Show the richest addresses")
	newUi("maxouts o", true, max_outs, "Show addresses with bniggest number of outputs")
	newUi("unspent u", false, list_unspent, "List balance of given bitcoin address")
}
