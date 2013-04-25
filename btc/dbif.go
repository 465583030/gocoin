package btc

import "fmt"

// Returned by GetUnspentFromPkScr
type OneUnspentTx struct {
	TxPrevOut
	Value uint64
	AskIndex uint
}

func (ou *OneUnspentTx) String() string {
	return fmt.Sprintf("%15.8f BTC from ", float64(ou.Value)/1e8) + ou.TxPrevOut.String()
}

type BlockChanges struct {
	Height uint32
	AddedTxs map[TxPrevOut] *TxOut
	DeledTxs map[TxPrevOut] *TxOut
}

type UnspentDB interface {
	CommitBlockTxs(*BlockChanges, []byte) error
	UndoBlockTransactions(uint32)
	GetLastBlockHash() []byte
	
	UnspentGet(out *TxPrevOut) (*TxOut, error)
	GetAllUnspent(addr []*BtcAddr) []OneUnspentTx

	Idle()
	Save()
	Close()
	NoSync()
	Sync()
	GetStats() (string)
}

var NewUnspentDb func(string, bool) UnspentDB
