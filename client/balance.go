package main

import (
	"io"
	"os"
	"fmt"
	"sort"
	"sync"
	"github.com/piotrnar/gocoin/btc"
)

var (
	mutex_bal sync.Mutex
	MyBalance btc.AllUnspentTx  // unspent outputs that can be removed
	MyWallet *oneWallet     // addresses that cann be poped up
	LastBalance uint64
	BalanceChanged bool
	BalanceInvalid bool = true
)


// This is called while accepting the block (from teh chain's thread)
func TxNotify (idx *btc.TxPrevOut, valpk *btc.TxOut) {
	if valpk!=nil {
		if MyWallet==nil {
			return
		}
		for i := range MyWallet.addrs {
			if MyWallet.addrs[i].Owns(valpk.Pk_script) {
				if dbg>0 {
					fmt.Println(" +", idx.String(), valpk.String(AddrVersion))
				}
				mutex_bal.Lock()
				MyBalance = append(MyBalance, btc.OneUnspentTx{TxPrevOut:*idx,
					Value:valpk.Value, MinedAt:valpk.BlockHeight, BtcAddr:MyWallet.addrs[i]})
				mutex_bal.Unlock()
				BalanceChanged = true
				break
			}
		}
	} else {
		mutex_bal.Lock()
		for i := range MyBalance {
			if MyBalance[i].TxPrevOut == *idx {
				tmp := make([]btc.OneUnspentTx, len(MyBalance)-1)
				if dbg>0 {
					fmt.Println(" -", MyBalance[i].String())
				}
				copy(tmp[:i], MyBalance[:i])
				copy(tmp[i:], MyBalance[i+1:])
				MyBalance = tmp
				BalanceChanged = true
				break
			}
		}
		mutex_bal.Unlock()
	}
}


func GetRawTransaction(BlockHeight uint32, txid *btc.Uint256, txf io.Writer) bool {
	// Find the block with the indicated Height in the main tree
	BlockChain.BlockIndexAccess.Lock()
	n := Last.Block
	if n.Height < BlockHeight {
		println(n.Height, BlockHeight)
		BlockChain.BlockIndexAccess.Unlock()
		panic("This should not happen")
	}
	for n.Height > BlockHeight {
		n = n.Parent
	}
	BlockChain.BlockIndexAccess.Unlock()

	bd, _, e := BlockChain.Blocks.BlockGet(n.BlockHash)
	if e != nil {
		println("BlockGet", n.BlockHash.String(), BlockHeight, e.Error())
		println("This should not happen - please, report a bug.")
		println("You can probably fix it by launching the client with -rescan")
		os.Exit(1)
	}

	bl, e := btc.NewBlock(bd)
	if e != nil {
		println("NewBlock: ", e.Error())
		os.Exit(1)
	}

	e = bl.BuildTxList()
	if e != nil {
		println("BuildTxList:", e.Error())
		os.Exit(1)
	}

	// Find the transaction we need and store it in the file
	for i := range bl.Txs {
		if bl.Txs[i].Hash.Equal(txid) {
			txf.Write(bl.Txs[i].Serialize())
			return true
		}
	}
	return false
}


// Call it only from the Chain thread
func DumpBalance(utxt *os.File, details bool) (s string) {
	var sum uint64
	mutex_bal.Lock()
	defer mutex_bal.Unlock()

	for i := range MyBalance {
		sum += MyBalance[i].Value

		if details {
			if i<100 {
				s += fmt.Sprintf("%7d %s\n", 1+Last.Block.Height-MyBalance[i].MinedAt,
					MyBalance[i].String())
			} else if i==100 {
				s += fmt.Sprintln("List of unspent outputs truncated to 100 records")
			}
		}

		// update the balance/ folder
		if utxt != nil {
			po, e := BlockChain.Unspent.UnspentGet(&MyBalance[i].TxPrevOut)
			if e != nil {
				println("UnspentGet:", e.Error())
				println("This should not happen - please, report a bug.")
				println("You can probably fix it by launching the client with -rescan")
				os.Exit(1)
			}

			txid := btc.NewUint256(MyBalance[i].TxPrevOut.Hash[:])

			// Store the unspent line in balance/unspent.txt
			fmt.Fprintf(utxt, "%s # %.8f BTC @ %s, %d confs\n", MyBalance[i].TxPrevOut.String(),
				float64(MyBalance[i].Value)/1e8, MyBalance[i].BtcAddr.StringLab(),
				1+Last.Block.Height-MyBalance[i].MinedAt)

			// store the entire transactiojn in balance/<txid>.tx
			fn := "balance/"+txid.String()[:64]+".tx"
			txf, _ := os.Open(fn)
			if txf == nil {
				// Do it only once per txid
				txf, _ = os.Create(fn)
				if txf==nil {
					println("Cannot create ", fn)
					os.Exit(1)
				}
				GetRawTransaction(po.BlockHeight, txid, txf)
			}
			txf.Close()
		}
	}
	LastBalance = sum
	s += fmt.Sprintf("Total balance: %.8f BTC in %d unspent outputs\n", float64(sum)/1e8, len(MyBalance))
	if utxt != nil {
		utxt.Close()
	}
	return
}


func show_balance(p string) {
	if p=="sum" {
		fmt.Print(DumpBalance(nil, false))
		return
	}
	if p!="" {
		fmt.Println("Using wallet from file", p, "...")
		LoadWallet(p)
	}

	if MyWallet==nil {
		println("You have no loaded wallet")
		return
	}

	if len(MyWallet.addrs)==0 {
		println("Your loaded wallet has no addresses")
		return
	}

	fmt.Print(UpdateBalanceFolder())
	fmt.Println("Your balance data has been saved to the 'balance/' folder.")
	fmt.Println("You nend to move this folder to your wallet PC, to spend the coins.")
}


func update_balance() {
	mutex_bal.Lock()
	MyBalance = BlockChain.GetAllUnspent(MyWallet.addrs, true)
	LastBalance = 0
	if len(MyBalance) > 0 {
		sort.Sort(MyBalance)
		for i := range MyBalance {
			LastBalance += MyBalance[i].Value
		}
	}
	BalanceInvalid = false
	mutex_bal.Unlock()
}


func UpdateBalanceFolder() string {
	os.RemoveAll("balance")
	os.MkdirAll("balance/", 0770)
	if BalanceInvalid {
		update_balance()
	}
	utxt, _ := os.Create("balance/unspent.txt")
	return DumpBalance(utxt, true)
}

func init() {
	newUi("balance bal", true, show_balance, "Show & save balance of currently loaded or a specified wallet")
}
