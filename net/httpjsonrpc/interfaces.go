package httpjsonrpc

import (
	"Elastos.ELA/account"
	. "Elastos.ELA/common"
	"Elastos.ELA/common/config"
	"Elastos.ELA/common/log"
	"Elastos.ELA/core/contract"
	"Elastos.ELA/core/contract/program"
	"Elastos.ELA/core/ledger"
	"Elastos.ELA/core/signature"
	"Elastos.ELA/core/transaction"
	tx "Elastos.ELA/core/transaction"
	"Elastos.ELA/core/transaction/payload"
	"Elastos.ELA/crypto"
	. "Elastos.ELA/errors"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"time"
)

const (
	AUXBLOCK_GENERATED_INTERVAL_SECONDS = 60
)

type BatchOut struct {
	Address string
	Value   string
}

type sortedCoinsItem struct {
	input *transaction.UTXOTxInput
	coin  *account.Coin
}

// sortedCoins used for spend minor coins first
type sortedCoins []*sortedCoinsItem

func (sc sortedCoins) Len() int      { return len(sc) }
func (sc sortedCoins) Swap(i, j int) { sc[i], sc[j] = sc[j], sc[i] }
func (sc sortedCoins) Less(i, j int) bool {
	if sc[i].coin.Output.Value > sc[j].coin.Output.Value {
		return false
	} else {
		return true
	}
}

var Wallet account.Client
var PreChainHeight uint64
var PreTime int64
var PreTransactionCount int

func TransArryByteToHexString(ptx *tx.Transaction) *Transactions {

	trans := new(Transactions)
	trans.TxType = ptx.TxType
	trans.PayloadVersion = ptx.PayloadVersion
	trans.Payload = TransPayloadToHex(ptx.Payload)

	n := 0
	trans.Attributes = make([]TxAttributeInfo, len(ptx.Attributes))
	for _, v := range ptx.Attributes {
		trans.Attributes[n].Usage = v.Usage
		trans.Attributes[n].Data = BytesToHexString(v.Data)
		n++
	}

	n = 0
	isCoinbase := ptx.IsCoinBaseTx()
	reference, _ := ptx.GetReference()
	trans.UTXOInputs = make([]UTXOTxInputInfo, len(ptx.UTXOInputs))
	for _, v := range ptx.UTXOInputs {
		trans.UTXOInputs[n].ReferTxID = BytesToHexString(v.ReferTxID.ToArrayReverse())
		trans.UTXOInputs[n].ReferTxOutputIndex = v.ReferTxOutputIndex
		trans.UTXOInputs[n].Sequence = v.Sequence
		if isCoinbase {
			trans.UTXOInputs[n].Address = ""
			trans.UTXOInputs[n].Value = ""
		} else {
			prevOutput := reference[v]
			trans.UTXOInputs[n].Address, _ = prevOutput.ProgramHash.ToAddress()
			trans.UTXOInputs[n].Value = prevOutput.Value.String()
		}
		n++
	}

	n = 0
	trans.BalanceInputs = make([]BalanceTxInputInfo, len(ptx.BalanceInputs))
	for _, v := range ptx.BalanceInputs {
		trans.BalanceInputs[n].AssetID = BytesToHexString(v.AssetID.ToArrayReverse())
		trans.BalanceInputs[n].Value = v.Value
		trans.BalanceInputs[n].ProgramHash = BytesToHexString(v.ProgramHash.ToArrayReverse())
		n++
	}

	n = 0
	trans.Outputs = make([]TxoutputInfo, len(ptx.Outputs))
	for _, v := range ptx.Outputs {
		trans.Outputs[n].AssetID = BytesToHexString(v.AssetID.ToArrayReverse())
		trans.Outputs[n].Value = v.Value.String()
		address, _ := v.ProgramHash.ToAddress()
		trans.Outputs[n].Address = address
		trans.Outputs[n].OutputLock = v.OutputLock
		n++
	}

	n = 0
	trans.Programs = make([]ProgramInfo, len(ptx.Programs))
	for _, v := range ptx.Programs {
		trans.Programs[n].Code = BytesToHexString(v.Code)
		trans.Programs[n].Parameter = BytesToHexString(v.Parameter)
		n++
	}

	n = 0
	trans.AssetOutputs = make([]TxoutputMap, len(ptx.AssetOutputs))
	for k, v := range ptx.AssetOutputs {
		trans.AssetOutputs[n].Key = k
		trans.AssetOutputs[n].Txout = make([]TxoutputInfo, len(v))
		for m := 0; m < len(v); m++ {
			trans.AssetOutputs[n].Txout[m].AssetID = BytesToHexString(v[m].AssetID.ToArrayReverse())
			trans.AssetOutputs[n].Txout[m].Value = v[m].Value.String()
			address, _ := v[m].ProgramHash.ToAddress()
			trans.AssetOutputs[n].Txout[m].Address = address
			trans.AssetOutputs[n].Txout[m].OutputLock = v[m].OutputLock
		}
		n += 1
	}

	trans.LockTime = ptx.LockTime

	n = 0
	trans.AssetInputAmount = make([]AmountMap, len(ptx.AssetInputAmount))
	for k, v := range ptx.AssetInputAmount {
		trans.AssetInputAmount[n].Key = k
		trans.AssetInputAmount[n].Value = v
		n += 1
	}

	n = 0
	trans.AssetOutputAmount = make([]AmountMap, len(ptx.AssetOutputAmount))
	for k, v := range ptx.AssetOutputAmount {
		trans.AssetInputAmount[n].Key = k
		trans.AssetInputAmount[n].Value = v
		n += 1
	}

	mHash := ptx.Hash()
	trans.Hash = BytesToHexString(mHash.ToArrayReverse())

	return trans
}

func getBestBlockHash(params []interface{}) map[string]interface{} {
	hash := ledger.DefaultLedger.Blockchain.CurrentBlockHash()
	return ElaRpc(BytesToHexString(hash.ToArrayReverse()))
}

// Input JSON string examples for getblock method as following:
//   {"jsonrpc": "2.0", "method": "getblock", "params": [1], "id": 0}
//   {"jsonrpc": "2.0", "method": "getblock", "params": ["aabbcc.."], "id": 0}
func getBlock(params []interface{}) map[string]interface{} {
	if len(params) < 1 {
		return ElaRpcNil
	}
	var err error
	var hash Uint256
	switch (params[0]).(type) {
	// block height
	case float64:
		index := uint32(params[0].(float64))
		hash, err = ledger.DefaultLedger.Store.GetBlockHash(index)
		if err != nil {
			return ElaRpcUnknownBlock
		}
		// block hash
	case string:
		str := params[0].(string)
		hex, err := HexStringToBytesReverse(str)
		if err != nil {
			return ElaRpcInvalidParameter
		}
		if err := hash.Deserialize(bytes.NewReader(hex)); err != nil {
			return ElaRpcInvalidTransaction
		}
	default:
		return ElaRpcInvalidParameter
	}

	block, err := ledger.DefaultLedger.Store.GetBlock(hash)
	if err != nil {
		return ElaRpcUnknownBlock
	}

	blockHead := &BlockHead{
		Version:          block.Blockdata.Version,
		PrevBlockHash:    BytesToHexString(block.Blockdata.PrevBlockHash.ToArrayReverse()),
		TransactionsRoot: BytesToHexString(block.Blockdata.TransactionsRoot.ToArrayReverse()),
		Timestamp:        block.Blockdata.Timestamp,
		Bits:             block.Blockdata.Bits,
		Height:           block.Blockdata.Height,
		Nonce:            block.Blockdata.Nonce,
		Hash:             BytesToHexString(hash.ToArrayReverse()),
	}

	trans := make([]*Transactions, len(block.Transactions))
	for i := 0; i < len(block.Transactions); i++ {
		trans[i] = TransArryByteToHexString(block.Transactions[i])
		trans[i].Timestamp = block.Blockdata.Timestamp
		trans[i].Confirminations = ledger.DefaultLedger.Blockchain.GetBestHeight() - block.Blockdata.Height + 1
		w := bytes.NewBuffer(nil)
		block.Transactions[i].Serialize(w)
		trans[i].TxSize = uint32(len(w.Bytes()))

	}

	coinbasePd := block.Transactions[0].Payload.(*payload.CoinBase)
	b := BlockInfo{
		Hash:            BytesToHexString(hash.ToArrayReverse()),
		BlockData:       blockHead,
		Transactions:    trans,
		Confirminations: ledger.DefaultLedger.Blockchain.GetBestHeight() - block.Blockdata.Height + 1,
		MinerInfo:       string(coinbasePd.CoinbaseData),
	}
	return ElaRpc(b)
}

func getBlockCount(params []interface{}) map[string]interface{} {
	return ElaRpc(ledger.DefaultLedger.Blockchain.BlockHeight + 1)
}

// A JSON example for getblockhash method as following:
//   {"jsonrpc": "2.0", "method": "getblockhash", "params": [1], "id": 0}
func getBlockHash(params []interface{}) map[string]interface{} {
	if len(params) < 1 {
		return ElaRpcNil
	}
	switch params[0].(type) {
	case float64:
		height := uint32(params[0].(float64))
		hash, err := ledger.DefaultLedger.Store.GetBlockHash(height)
		if err != nil {
			return ElaRpcUnknownBlock
		}
		return ElaRpc(BytesToHexString(hash.ToArrayReverse()))
	default:
		return ElaRpcInvalidParameter
	}
}

func getConnectionCount(params []interface{}) map[string]interface{} {
	return ElaRpc(node.GetConnectionCnt())
}

func getRawMemPool(params []interface{}) map[string]interface{} {
	txs := []*Transactions{}
	txpool := node.GetTxnPool(false)
	for _, t := range txpool {
		txs = append(txs, TransArryByteToHexString(t))
	}
	if len(txs) == 0 {
		return ElaRpcNil
	}
	return ElaRpc(txs)
}

// A JSON example for getrawtransaction method as following:
//   {"jsonrpc": "2.0", "method": "getrawtransaction", "params": ["transactioin hash in hex"], "id": 0}
func getRawTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 1 {
		return ElaRpcNil
	}
	switch params[0].(type) {
	case string:
		str := params[0].(string)
		hex, err := HexStringToBytesReverse(str)
		if err != nil {
			return ElaRpcInvalidParameter
		}
		var hash Uint256
		err = hash.Deserialize(bytes.NewReader(hex))
		if err != nil {
			return ElaRpcInvalidTransaction
		}
		tx, height, err := ledger.DefaultLedger.Store.GetTransaction(hash)
		if err != nil {
			return ElaRpcUnknownTransaction
		}
		bHash, err := ledger.DefaultLedger.Store.GetBlockHash(height)
		if err != nil {
			return ElaRpcUnknownTransaction
		}
		header, err := ledger.DefaultLedger.Store.GetHeader(bHash)
		if err != nil {
			return ElaRpcUnknownTransaction
		}
		tran := TransArryByteToHexString(tx)
		tran.Timestamp = header.Blockdata.Timestamp
		tran.Confirminations = ledger.DefaultLedger.Blockchain.GetBestHeight() - height + 1
		w := bytes.NewBuffer(nil)
		tx.Serialize(w)
		tran.TxSize = uint32(len(w.Bytes()))

		return ElaRpc(tran)
	default:
		return ElaRpcInvalidParameter
	}
}

func getNeighbor(params []interface{}) map[string]interface{} {
	addr, _ := node.GetNeighborAddrs()
	return ElaRpc(addr)
}

func getNodeState(params []interface{}) map[string]interface{} {
	n := NodeInfo{
		State:    uint(node.GetState()),
		Time:     node.GetTime(),
		Port:     node.GetPort(),
		ID:       node.GetID(),
		Version:  node.Version(),
		Services: node.Services(),
		Relay:    node.GetRelay(),
		Height:   node.GetHeight(),
		TxnCnt:   node.GetTxnCnt(),
		RxTxnCnt: node.GetRxTxnCnt(),
	}
	return ElaRpc(n)
}

func setDebugInfo(params []interface{}) map[string]interface{} {
	if len(params) < 1 {
		return ElaRpcInvalidParameter
	}
	switch params[0].(type) {
	case float64:
		level := params[0].(float64)
		if err := log.Log.SetDebugLevel(int(level)); err != nil {
			return ElaRpcInvalidParameter
		}
	default:
		return ElaRpcInvalidParameter
	}
	return ElaRpcSuccess
}

func submitAuxBlock(params []interface{}) map[string]interface{} {
	auxPow, blockHash := "", ""
	switch params[0].(type) {
	case string:
		blockHash = params[0].(string)
		if _, ok := Pow.MsgBlock.BlockData[blockHash]; !ok {
			log.Trace("[json-rpc:submitAuxBlock] receive invalid block hash value:", blockHash)
			return ElaRpcInvalidHash
		}

	default:
		return ElaRpcInvalidParameter
	}

	switch params[1].(type) {
	case string:
		auxPow = params[1].(string)
		temp, _ := HexStringToBytes(auxPow)
		r := bytes.NewBuffer(temp)
		Pow.MsgBlock.BlockData[blockHash].Blockdata.AuxPow.Deserialize(r)
		_, _, err := ledger.DefaultLedger.Blockchain.AddBlock(Pow.MsgBlock.BlockData[blockHash])
		if err != nil {
			log.Trace(err)
			return ElaRpcInternalError
		}

		Pow.MsgBlock.Mutex.Lock()
		for key := range Pow.MsgBlock.BlockData {
			delete(Pow.MsgBlock.BlockData, key)
		}
		Pow.MsgBlock.Mutex.Unlock()
		log.Trace("AddBlock called finished and Pow.MsgBlock.BlockData has been deleted completely")

	default:
		return ElaRpcInvalidParameter
	}
	log.Info(auxPow, blockHash)
	return ElaRpcSuccess
}

func generateAuxBlock(addr string) (*ledger.Block, string, bool) {
	msgBlock := &ledger.Block{}

	if node.GetHeight() == 0 || PreChainHeight != node.GetHeight() || (time.Now().Unix()-PreTime > AUXBLOCK_GENERATED_INTERVAL_SECONDS && Pow.GetTransactionCount() != PreTransactionCount) {
		if PreChainHeight != node.GetHeight() {
			PreChainHeight = node.GetHeight()
			PreTime = time.Now().Unix()
			PreTransactionCount = Pow.GetTransactionCount()
		}

		currentTxsCount := Pow.CollectTransactions(msgBlock)
		if 0 == currentTxsCount {
			return nil, "currentTxs is nil", false
		}

		msgBlock, err := Pow.GenerateBlock(addr)
		if nil != err {
			return nil, "msgBlock generate err", false
		}

		curHash := msgBlock.Hash()
		curHashStr := BytesToHexString(curHash.ToArray())

		Pow.MsgBlock.Mutex.Lock()
		Pow.MsgBlock.BlockData[curHashStr] = msgBlock
		Pow.MsgBlock.Mutex.Unlock()

		PreChainHeight = node.GetHeight()
		PreTime = time.Now().Unix()
		PreTransactionCount = currentTxsCount // Don't Call GetTransactionCount()

		return msgBlock, curHashStr, true
	}
	return nil, "", false
}

func createAuxBlock(params []interface{}) map[string]interface{} {
	msgBlock, curHashStr, _ := generateAuxBlock(config.Parameters.PowConfiguration.PayToAddr)
	if nil == msgBlock {
		return ElaRpcNil
	}

	type AuxBlock struct {
		ChainId           int    `json:"chainid"`
		Height            uint64 `json:"height"`
		CoinBaseValue     int    `json:"coinbasevalue"`
		Bits              string `json:"bits"`
		Hash              string `json:"hash"`
		PreviousBlockHash string `json:"previousblockhash"`
	}

	switch params[0].(type) {
	case string:
		Pow.PayToAddr = params[0].(string)

		preHash := ledger.DefaultLedger.Blockchain.CurrentBlockHash()
		preHashStr := BytesToHexString(preHash.ToArray())

		SendToAux := AuxBlock{
			ChainId:           1,
			Height:            node.GetHeight(),
			CoinBaseValue:     1,                                          //transaction content
			Bits:              fmt.Sprintf("%x", msgBlock.Blockdata.Bits), //difficulty
			Hash:              curHashStr,
			PreviousBlockHash: preHashStr}
		return ElaRpc(&SendToAux)

	default:
		return ElaRpcInvalidParameter

	}
}

func getInfo(params []interface{}) map[string]interface{} {
	RetVal := struct {
		Version     int    `json:"version"`
		Balance     int    `json:"balance"`
		Blocks      uint64 `json:"blocks"`
		Timeoffset  int    `json:"timeoffset"`
		Connections uint   `json:"connections"`
		//Difficulty      int    `json:"difficulty"`
		Testnet        bool   `json:"testnet"`
		Keypoololdest  int    `json:"keypoololdest"`
		Keypoolsize    int    `json:"keypoolsize"`
		Unlocked_until int    `json:"unlocked_until"`
		Paytxfee       int    `json:"paytxfee"`
		Relayfee       int    `json:"relayfee"`
		Errors         string `json:"errors"`
	}{
		Version:     config.Parameters.Version,
		Balance:     0,
		Blocks:      node.GetHeight(),
		Timeoffset:  0,
		Connections: node.GetConnectionCnt(),
		//Difficulty:      ledger.PowLimitBits,
		Testnet:        config.Parameters.PowConfiguration.TestNet,
		Keypoololdest:  0,
		Keypoolsize:    0,
		Unlocked_until: 0,
		Paytxfee:       0,
		Relayfee:       0,
		Errors:         "Tobe written"}
	return ElaRpc(&RetVal)
}

func auxHelp(params []interface{}) map[string]interface{} {

	//TODO  and description for this rpc-interface
	return ElaRpc("createauxblock==submitauxblock")
}

func getVersion(params []interface{}) map[string]interface{} {
	return ElaRpc(config.Version)
}

func addAccount(params []interface{}) map[string]interface{} {
	if Wallet == nil {
		return ElaRpc("open wallet first")
	}
	account, err := Wallet.CreateAccount()
	if err != nil {
		return ElaRpc("create account error:" + err.Error())
	}

	if err := Wallet.CreateContract(account); err != nil {
		return ElaRpc("create contract error:" + err.Error())
	}

	address, err := account.ProgramHash.ToAddress()
	if err != nil {
		return ElaRpc("generate address error:" + err.Error())
	}

	return ElaRpc(address)
}

func deleteAccount(params []interface{}) map[string]interface{} {
	if len(params) < 1 {
		return ElaRpcNil
	}
	var address string
	switch params[0].(type) {
	case string:
		address = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}
	if Wallet == nil {
		return ElaRpc("open wallet first")
	}
	programHash, err := ToScriptHash(address)
	if err != nil {
		return ElaRpc("invalid address:" + err.Error())
	}
	if err := Wallet.DeleteAccount(programHash); err != nil {
		return ElaRpc("Delete account error:" + err.Error())
	}
	if err := Wallet.DeleteContract(programHash); err != nil {
		return ElaRpc("Delete contract error:" + err.Error())
	}
	if err := Wallet.DeleteCoinsData(programHash); err != nil {
		return ElaRpc("Delete coins error:" + err.Error())
	}

	return ElaRpc(true)
}

func toggleCpuMining(params []interface{}) map[string]interface{} {
	var isMining bool
	switch params[0].(type) {
	case bool:
		isMining = params[0].(bool)

	default:
		return ElaRpcInvalidParameter
	}

	if isMining {
		go Pow.Start()
	} else {
		go Pow.Halt()
	}

	return ElaRpcSuccess
}

func manualCpuMining(params []interface{}) map[string]interface{} {
	var numBlocks uint32
	switch params[0].(type) {
	case float64:
		numBlocks = uint32(params[0].(float64))
	default:
		return ElaRpcInvalidParameter
	}

	if numBlocks == 0 {
		return ElaRpcInvalidParameter
	}

	ret := make([]string, numBlocks)

	blockHashes, err := Pow.ManualMining(numBlocks)
	if err != nil {
		return ElaRpcFailed
	}

	for i, hash := range blockHashes {
		//ret[i] = hash.ToString()
		w := bytes.NewBuffer(nil)
		hash.Serialize(w)
		ret[i] = BytesToHexString(w.Bytes())
	}

	return ElaRpc(ret)
}

func sendTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 4 {
		return ElaRpcNil
	}

	var asset, address, value, fee, utxolock string
	switch params[0].(type) {
	case string:
		asset = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[1].(type) {
	case string:
		address = params[1].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[2].(type) {
	case string:
		value = params[2].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[3].(type) {
	case string:
		fee = params[3].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[4].(type) {
	case string:
		utxolock = params[4].(string)
	default:
		return ElaRpcInvalidParameter
	}
	if Wallet == nil {
		return ElaRpc("error : wallet is not opened")
	}

	batchOut := BatchOut{
		Address: address,
		Value:   value,
	}
	tmp, err := HexStringToBytesReverse(asset)
	if err != nil {
		return ElaRpc("error: invalid asset ID")
	}
	var assetID Uint256
	if err := assetID.Deserialize(bytes.NewReader(tmp)); err != nil {
		return ElaRpc("error: invalid asset hash")
	}
	txn, err := MakeTransferTransaction(Wallet, assetID, fee, utxolock, batchOut)
	if err != nil {
		return ElaRpc("error: " + err.Error())
	}

	if errCode := VerifyAndSendTx(txn); errCode != Success {
		return ElaRpc("error: " + errCode.Error())
	}
	txHash := txn.Hash()
	return ElaRpc(BytesToHexString(txHash.ToArrayReverse()))
}

func sendBatchOutTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 3 {
		return ElaRpcNil
	}
	var asset, fee, utxolock string
	var batchOutArray []interface{}
	switch params[0].(type) {
	case string:
		asset = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[1].(type) {
	case []interface{}:
		batchOutArray = params[1].([]interface{})
	default:
		return ElaRpcInvalidParameter
	}
	switch params[2].(type) {
	case string:
		fee = params[2].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[3].(type) {
	case string:
		utxolock = params[3].(string)
	default:
		return ElaRpcInvalidParameter
	}
	if Wallet == nil {
		return ElaRpc("error : wallet is not opened")
	}

	content, err := json.Marshal(batchOutArray)
	if err != nil {
		return ElaRpc("error : batch out marshal failed")
	}
	batchOut := []BatchOut{}
	err = json.Unmarshal(content, &batchOut)
	if err != nil {
		return ElaRpc("error : batch out unmarshal failed")
	}

	tmp, err := HexStringToBytesReverse(asset)
	if err != nil {
		return ElaRpc("error: invalid asset ID")
	}
	var assetID Uint256
	if err := assetID.Deserialize(bytes.NewReader(tmp)); err != nil {
		return ElaRpc("error: invalid asset hash")
	}
	txn, err := MakeTransferTransaction(Wallet, assetID, fee, utxolock, batchOut...)
	if err != nil {
		return ElaRpc("error: " + err.Error())
	}

	if errCode := VerifyAndSendTx(txn); errCode != Success {
		return ElaRpc("error: " + errCode.Error())
	}
	txHash := txn.Hash()
	return ElaRpc(BytesToHexString(txHash.ToArrayReverse()))
}

// A JSON example for sendrawtransaction method as following:
//   {"jsonrpc": "2.0", "method": "sendrawtransaction", "params": ["raw transactioin in hex"], "id": 0}
func sendRawTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 1 {
		return ElaRpcNil
	}
	var hash Uint256
	switch params[0].(type) {
	case string:
		str := params[0].(string)
		hex, err := HexStringToBytes(str)
		if err != nil {
			return ElaRpcInvalidParameter
		}
		var txn tx.Transaction
		if err := txn.Deserialize(bytes.NewReader(hex)); err != nil {
			return ElaRpcInvalidTransaction
		}
		hash = txn.Hash()
		if errCode := VerifyAndSendTx(&txn); errCode != Success {
			return ElaRpc(errCode.Error())
		}
	default:
		return ElaRpcInvalidParameter
	}
	return ElaRpc(BytesToHexString(hash.ToArrayReverse()))
}

// A JSON example for submitblock method as following:
//   {"jsonrpc": "2.0", "method": "submitblock", "params": ["raw block in hex"], "id": 0}
func submitBlock(params []interface{}) map[string]interface{} {
	if len(params) < 1 {
		return ElaRpcNil
	}
	switch params[0].(type) {
	case string:
		str := params[0].(string)
		hex, _ := HexStringToBytes(str)
		var block ledger.Block
		if err := block.Deserialize(bytes.NewReader(hex)); err != nil {
			return ElaRpcInvalidBlock
		}
		if _, _, err := ledger.DefaultLedger.Blockchain.AddBlock(&block); err != nil {
			return ElaRpcInvalidBlock
		}
		if err := node.Xmit(&block); err != nil {
			return ElaRpcInternalError
		}
	default:
		return ElaRpcInvalidParameter
	}
	return ElaRpcSuccess
}

func signMultiSignTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 1 {
		return ElaRpcNil
	}
	var signedrawtxn string
	switch params[0].(type) {
	case string:
		signedrawtxn = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}

	rawtxn, _ := HexStringToBytes(signedrawtxn)
	var txn tx.Transaction
	txn.Deserialize(bytes.NewReader(rawtxn))
	if len(txn.Programs) <= 0 {
		return ElaRpc("error: missing the first signature")
	}

	_, needSign, err := txn.ParseTransactionSig()
	if err != nil {
		return ElaRpc("error: " + err.Error())
	}

	if needSign > 0 {
		var acct *account.Account
		programHashes := txn.ParseTransactionCode()
		for _, hash := range programHashes {
			acct = Wallet.GetAccountByProgramHash(hash)
			if acct != nil {
				break
			}
		}

		if acct == nil {
			return ElaRpc("error: no available account detected")
		} else {
			sig, _ := signature.SignBySigner(&txn, acct)
			txn.AppendNewSignature(sig)
		}
	}

	var buffer bytes.Buffer
	txn.Serialize(&buffer)
	return ElaRpc(BytesToHexString(buffer.Bytes()))
}

func createMultiSignTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 5 {
		return ElaRpcNil
	}
	var asset, from, address, value, fee string
	switch params[0].(type) {
	case string:
		asset = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[1].(type) {
	case string:
		from = params[1].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[2].(type) {
	case string:
		address = params[2].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[3].(type) {
	case string:
		value = params[3].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[4].(type) {
	case string:
		fee = params[4].(string)
	default:
		return ElaRpcInvalidParameter
	}

	if Wallet == nil {
		return ElaRpc("error : wallet is not opened")
	}

	batchOut := BatchOut{
		Address: address,
		Value:   value,
	}
	tmp, err := HexStringToBytesReverse(asset)
	if err != nil {
		return ElaRpc("error: invalid asset ID")
	}
	var assetID Uint256
	if err := assetID.Deserialize(bytes.NewReader(tmp)); err != nil {
		return ElaRpc("error: invalid asset hash")
	}
	txn, err := MakeMultisigTransferTransaction(Wallet, assetID, from, fee, batchOut)
	if err != nil {
		return ElaRpc("error:" + err.Error())
	}

	var buffer bytes.Buffer
	txn.Serialize(&buffer)
	return ElaRpc(BytesToHexString(buffer.Bytes()))
}
func depositunlockTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 7 {
		return ElaRpcNil
	}
	var asset, from, address, key, value, fee, s string
	switch params[0].(type) {
	case string:
		asset = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[1].(type) {
	case string:
		from = params[1].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[2].(type) {
	case string:
		address = params[2].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[3].(type) {
	case string:
		key = params[3].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[4].(type) {
	case string:
		value = params[4].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[5].(type) {
	case string:
		fee = params[5].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[6].(type) {
	case string:
		s = params[6].(string)
	default:
		return ElaRpcInvalidParameter
	}

	if Wallet == nil {
		return ElaRpc("error : wallet is not opened")
	}

	batchOut := BatchOut{
		Address: address,
		Value:   value,
	}
	tmp, err := HexStringToBytesReverse(asset)
	if err != nil {
		return ElaRpc("error: invalid asset ID")
	}
	var assetID Uint256
	if err := assetID.Deserialize(bytes.NewReader(tmp)); err != nil {
		return ElaRpc("error: invalid asset hash")
	}
	txn, err := MakeScriptTransferTransaction(Wallet, assetID, from, fee, batchOut)
	if err != nil {
		return ElaRpc("error:" + err.Error())
	}
	//append code and parameter
	byteKey, err := HexStringToBytes(key)
	if err != nil {
		fmt.Print("error: invalid public key")
		return nil
	}
	rawKey, err := crypto.DecodePoint(byteKey)
	if err != nil {
		fmt.Print("error: invalid encoded public key")
		return nil
	}
	programs := make([]*program.Program, 1)
	hash, _ := HexStringToBytes(s)
	code, err := contract.CreateUnlockScriptRedeemScript(hash, rawKey, 100)
	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		return nil
	}
	fmt.Printf("Code: %s\n", BytesToHexString(code))
	programs[0] = &program.Program{
		Code:      code,
		Parameter: []byte{0},
	}
	txn.SetPrograms(programs)

	var buffer bytes.Buffer
	txn.Serialize(&buffer)
	return ElaRpc(BytesToHexString(buffer.Bytes()))
}

func withdrawTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 8 {
		return ElaRpcNil
	}
	var asset, from, address, keyA, keyS, value, fee, s string
	switch params[0].(type) {
	case string:
		asset = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[1].(type) {
	case string:
		from = params[1].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[2].(type) {
	case string:
		address = params[2].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[3].(type) {
	case string:
		keyA = params[3].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[4].(type) {
	case string:
		keyS = params[4].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[5].(type) {
	case string:
		value = params[5].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[6].(type) {
	case string:
		fee = params[6].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[7].(type) {
	case string:
		s = params[7].(string)
	default:
		return ElaRpcInvalidParameter
	}

	if Wallet == nil {
		return ElaRpc("error : wallet is not opened")
	}

	batchOut := BatchOut{
		Address: address,
		Value:   value,
	}
	tmp, err := HexStringToBytesReverse(asset)
	if err != nil {
		return ElaRpc("error: invalid asset ID")
	}
	var assetID Uint256
	if err := assetID.Deserialize(bytes.NewReader(tmp)); err != nil {
		return ElaRpc("error: invalid asset hash")
	}
	txn, err := MakeScriptTransferTransaction(Wallet, assetID, from, fee, batchOut)
	if err != nil {
		return ElaRpc("error:" + err.Error())
	}
	//append code and parameter
	byteKeyA, err := HexStringToBytes(keyA)
	if err != nil {
		fmt.Print("error: invalid public key")
		return nil
	}
	rawKeyA, err := crypto.DecodePoint(byteKeyA)
	if err != nil {
		fmt.Print("error: invalid encoded public key")
		return nil
	}
	byteKeyS, err := HexStringToBytes(keyS)
	if err != nil {
		fmt.Print("error: invalid public key")
		return nil
	}
	rawKeyS, err := crypto.DecodePoint(byteKeyS)
	if err != nil {
		fmt.Print("error: invalid encoded public key")
		return nil
	}
	programs := make([]*program.Program, 1)
	hash, _ := HexStringToBytes(s)
	code, err := contract.CreateWithdrawScriptRedeemScript(hash, rawKeyA, rawKeyS, 100)
	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		return nil
	}
	fmt.Printf("Code: %s\n", BytesToHexString(code))
	programs[0] = &program.Program{
		Code:      code,
		Parameter: []byte{0},
	}
	txn.SetPrograms(programs)

	var buffer bytes.Buffer
	txn.Serialize(&buffer)
	return ElaRpc(BytesToHexString(buffer.Bytes()))
}

//destroy token or refund token
func withdrawunlockTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 8 {
		return ElaRpcInvalidParameter
	}
	var asset, from, address, keyA, keyS, value, fee, s string
	switch params[0].(type) {
	case string:
		asset = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[1].(type) {
	case string:
		from = params[1].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[2].(type) {
	case string:
		address = params[2].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[3].(type) {
	case string:
		keyA = params[3].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[4].(type) {
	case string:
		keyS = params[4].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[5].(type) {
	case string:
		value = params[5].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[6].(type) {
	case string:
		fee = params[6].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[7].(type) {
	case string:
		s = params[7].(string)
	default:
		return ElaRpcInvalidParameter
	}

	if Wallet == nil {
		return ElaRpc("error : wallet is not opened")
	}

	batchOut := BatchOut{
		Address: address,
		Value:   value,
	}
	tmp, err := HexStringToBytesReverse(asset)
	if err != nil {
		return ElaRpc("error: invalid asset ID")
	}
	var assetID Uint256
	if err := assetID.Deserialize(bytes.NewReader(tmp)); err != nil {
		return ElaRpc("error: invalid asset hash")
	}
	txn, err := MakeScriptTransferTransaction(Wallet, assetID, from, fee, batchOut)
	if err != nil {
		return ElaRpc("error:" + err.Error())
	}
	//append code and parameter
	byteKeyA, err := HexStringToBytes(keyA)
	if err != nil {
		fmt.Print("error: invalid public key")
		return nil
	}
	rawKeyA, err := crypto.DecodePoint(byteKeyA)
	if err != nil {
		fmt.Print("error: invalid encoded public key")
		return nil
	}
	byteKeyS, err := HexStringToBytes(keyS)
	if err != nil {
		fmt.Print("error: invalid public key")
		return nil
	}
	rawKeyS, err := crypto.DecodePoint(byteKeyS)
	if err != nil {
		fmt.Print("error: invalid encoded public key")
		return nil
	}
	programs := make([]*program.Program, 1)
	hash, _ := HexStringToBytes(s)
	code, err := contract.CreateWithdrawUnlockScriptRedeemScript(hash, rawKeyS, rawKeyA, 1000)
	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		return nil
	}

	fmt.Printf("Code: %s\n", BytesToHexString(code))
	programs[0] = &program.Program{
		Code:      code,
		Parameter: []byte{0},
	}
	txn.SetPrograms(programs)

	var buffer bytes.Buffer
	txn.Serialize(&buffer)
	return ElaRpc(BytesToHexString(buffer.Bytes()))
}

func deposittosideTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 8 {
		return ElaRpcInvalidParameter
	}
	var asset, from, address, keyA, keyS, value, fee, s string
	switch params[0].(type) {
	case string:
		asset = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[1].(type) {
	case string:
		from = params[1].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[2].(type) {
	case string:
		address = params[2].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[3].(type) {
	case string:
		keyA = params[3].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[4].(type) {
	case string:
		keyS = params[4].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[5].(type) {
	case string:
		value = params[5].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[6].(type) {
	case string:
		fee = params[6].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[7].(type) {
	case string:
		s = params[7].(string)
	default:
		return ElaRpcInvalidParameter
	}

	if Wallet == nil {
		return ElaRpc("error : wallet is not opened")
	}

	batchOut := BatchOut{
		Address: address,
		Value:   value,
	}
	tmp, err := HexStringToBytesReverse(asset)
	if err != nil {
		return ElaRpc("error: invalid asset ID")
	}
	var assetID Uint256
	if err := assetID.Deserialize(bytes.NewReader(tmp)); err != nil {
		return ElaRpc("error: invalid asset hash")
	}
	txn, err := MakeScriptTransferTransaction(Wallet, assetID, from, fee, batchOut)
	if err != nil {
		return ElaRpc("error:" + err.Error())
	}
	//append code and parameter
	byteKeyA, err := HexStringToBytes(keyA)
	if err != nil {
		fmt.Print("error: invalid public key")
		return nil
	}
	rawKeyA, err := crypto.DecodePoint(byteKeyA)
	if err != nil {
		fmt.Print("error: invalid encoded public key")
		return nil
	}
	byteKeyS, err := HexStringToBytes(keyS)
	if err != nil {
		fmt.Print("error: invalid public key")
		return nil
	}
	rawKeyS, err := crypto.DecodePoint(byteKeyS)
	if err != nil {
		fmt.Print("error: invalid encoded public key")
		return nil
	}

	programs := make([]*program.Program, 1)
	hash, _ := HexStringToBytes(s)
	code, err := contract.CreateDepositScriptRedeemScript(hash, rawKeyS, rawKeyA, 1000)
	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		return nil
	}

	fmt.Printf("Code: %s\n", BytesToHexString(code))
	programs[0] = &program.Program{
		Code:      code,
		Parameter: []byte{0},
	}
	txn.SetPrograms(programs)

	var buffer bytes.Buffer
	txn.Serialize(&buffer)
	return ElaRpc(BytesToHexString(buffer.Bytes()))
}

func MakeScriptTransferTransaction(wallet account.Client, assetID Uint256, from string, fee string, batchOut ...BatchOut) (*transaction.Transaction, error) {
	outputNum := len(batchOut)
	if outputNum == 0 {
		return nil, errors.New("nil outputs")
	}

	spendAddress, err := ToScriptHash(from)
	if err != nil {
		return nil, errors.New("invalid sender address")
	}

	var expected Fixed64
	input := []*transaction.UTXOTxInput{}
	output := []*transaction.TxOutput{}
	txnfee, err := StringToFixed64(fee)
	if err != nil || txnfee <= 0 {
		return nil, errors.New("invalid transation fee")
	}
	expected += txnfee
	// construct transaction outputs
	for _, o := range batchOut {
		outputValue, err := StringToFixed64(o.Value)
		if err != nil {
			return nil, err
		}

		expected += outputValue
		address, err := ToScriptHash(o.Address)
		if err != nil {
			return nil, errors.New("invalid receiver address")
		}
		tmp := &transaction.TxOutput{
			AssetID:     assetID,
			Value:       outputValue,
			ProgramHash: address,
		}
		output = append(output, tmp)
	}
	log.Debug("expected = %v\n", expected)
	// construct transaction inputs and changes
	coins := wallet.GetCoins()
	sorted := sortAvailableCoinsByValue(coins, account.Script)
	for _, coinItem := range sorted {
		if coinItem.coin.Output.AssetID == assetID && coinItem.coin.Output.ProgramHash == spendAddress {
			input = append(input, coinItem.input)
			log.Debug("coinItem.coin.Output.Value = %v ProgramHash = %x\n", coinItem.coin.Output.Value, spendAddress.ToArrayReverse())
			if coinItem.coin.Output.Value > expected {
				changes := &transaction.TxOutput{
					AssetID:     assetID,
					Value:       coinItem.coin.Output.Value - expected,
					ProgramHash: spendAddress,
				}
				// if any, the changes output of transaction will be the last one
				output = append(output, changes)
				expected = 0
				break
			} else if coinItem.coin.Output.Value == expected {
				expected = 0
				break
			} else if coinItem.coin.Output.Value < expected {
				expected = expected - coinItem.coin.Output.Value
				fmt.Printf("expected - coinItem.coin.Output.Value = %v\n", expected)
			}
		}
	}
	if expected > 0 {
		return nil, errors.New("available token is not enough")
	}

	// construct transaction
	txn, err := transaction.NewTransferAssetTransaction(input, output)
	if err != nil {
		return nil, err
	}
	txAttr := transaction.NewTxAttribute(transaction.Nonce, []byte(strconv.FormatInt(rand.Int63(), 10)))
	txn.Attributes = make([]*transaction.TxAttribute, 0)
	txn.Attributes = append(txn.Attributes, &txAttr)

	return txn, nil
}

func createBatchOutMultiSignTransaction(params []interface{}) map[string]interface{} {
	if len(params) < 4 {
		return ElaRpcNil
	}
	var asset, from, fee string
	var batchOutArray []interface{}
	switch params[0].(type) {
	case string:
		asset = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[1].(type) {
	case string:
		from = params[1].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[2].(type) {
	case []interface{}:
		batchOutArray = params[2].([]interface{})
	default:
		return ElaRpcInvalidParameter
	}
	switch params[3].(type) {
	case string:
		fee = params[3].(string)
	default:
		return ElaRpcInvalidParameter
	}
	if Wallet == nil {
		return ElaRpc("error : wallet is not opened")
	}

	content, err := json.Marshal(batchOutArray)
	if err != nil {
		return ElaRpc("error : batch out marshal failed")
	}
	batchOut := []BatchOut{}
	err = json.Unmarshal(content, &batchOut)
	if err != nil {
		return ElaRpc("error : batch out unmarshal failed")
	}

	tmp, err := HexStringToBytesReverse(asset)
	if err != nil {
		return ElaRpc("error: invalid asset ID")
	}
	var assetID Uint256
	if err := assetID.Deserialize(bytes.NewReader(tmp)); err != nil {
		return ElaRpc("error: invalid asset hash")
	}

	txn, err := MakeMultisigTransferTransaction(Wallet, assetID, from, fee, batchOut...)
	if err != nil {
		return ElaRpc("error:" + err.Error())
	}

	var buffer bytes.Buffer
	txn.Serialize(&buffer)
	return ElaRpc(BytesToHexString(buffer.Bytes()))
}

func MakeTransferTransaction(wallet account.Client, assetID Uint256, fee string, lock string, batchOut ...BatchOut) (*transaction.Transaction, error) {
	// get main account which is used to receive changes
	mainAccount, err := wallet.GetDefaultAccount()
	if err != nil {
		return nil, err
	}
	utxolock, err := strconv.ParseUint(lock, 10, 32)
	if err != nil {
		return nil, err
	}
	// construct transaction outputs
	var expected Fixed64
	input := []*transaction.UTXOTxInput{}
	output := []*transaction.TxOutput{}
	txnfee, err := StringToFixed64(fee)
	if err != nil || txnfee <= 0 {
		return nil, errors.New("invalid transation fee")
	}
	expected += txnfee
	for _, o := range batchOut {
		outputValue, err := StringToFixed64(o.Value)
		if err != nil {
			return nil, err
		}
		expected += outputValue
		address, err := ToScriptHash(o.Address)
		if err != nil {
			return nil, errors.New("invalid address")
		}
		tmp := &transaction.TxOutput{
			AssetID:     assetID,
			Value:       outputValue,
			OutputLock:  uint32(utxolock),
			ProgramHash: address,
		}
		output = append(output, tmp)
	}

	// construct transaction inputs and changes
	coins := wallet.GetCoins()
	sorted := sortAvailableCoinsByValue(coins, account.SingleSign)
	for _, coinItem := range sorted {
		if coinItem.coin.Output.AssetID == assetID {
			if coinItem.coin.Output.OutputLock > 0 {
				//can not unlock
				if ledger.DefaultLedger.Blockchain.GetBestHeight() < coinItem.coin.Output.OutputLock {
					continue
				}
				//spend locked utxo,change the  input Sequence
				coinItem.input.Sequence = math.MaxUint32 - 1
			}
			input = append(input, coinItem.input)
			if coinItem.coin.Output.Value > expected {
				changes := &transaction.TxOutput{
					AssetID:     assetID,
					Value:       coinItem.coin.Output.Value - expected,
					OutputLock:  0,
					ProgramHash: mainAccount.ProgramHash,
				}
				// if any, the changes output of transaction will be the last one
				output = append(output, changes)
				expected = 0
				break
			} else if coinItem.coin.Output.Value == expected {
				expected = 0
				break
			} else if coinItem.coin.Output.Value < expected {
				expected = expected - coinItem.coin.Output.Value
			}
		}
	}
	if expected > 0 {
		return nil, errors.New("available token is not enough")
	}

	// construct transaction
	txn, err := transaction.NewTransferAssetTransaction(input, output)
	if err != nil {
		return nil, err
	}
	txn.LockTime = ledger.DefaultLedger.Blockchain.GetBestHeight()

	txAttr := transaction.NewTxAttribute(transaction.Nonce, []byte(strconv.FormatInt(rand.Int63(), 10)))
	txn.Attributes = make([]*transaction.TxAttribute, 0)
	txn.Attributes = append(txn.Attributes, &txAttr)

	// sign transaction contract
	ctx := contract.NewContractContext(txn)
	wallet.Sign(ctx)
	txn.SetPrograms(ctx.GetPrograms())

	return txn, nil
}

func MakeMultisigTransferTransaction(wallet account.Client, assetID Uint256, from string, fee string, batchOut ...BatchOut) (*transaction.Transaction, error) {
	//TODO: check if being transferred asset is System Token(IPT)
	outputNum := len(batchOut)
	if outputNum == 0 {
		return nil, errors.New("nil outputs")
	}

	spendAddress, err := ToScriptHash(from)
	if err != nil {
		return nil, errors.New("invalid sender address")
	}

	var expected Fixed64
	input := []*transaction.UTXOTxInput{}
	output := []*transaction.TxOutput{}
	txnfee, err := StringToFixed64(fee)
	if err != nil || txnfee <= 0 {
		return nil, errors.New("invalid transation fee")
	}
	expected += txnfee
	// construct transaction outputs
	for _, o := range batchOut {
		outputValue, err := StringToFixed64(o.Value)
		if err != nil {
			return nil, err
		}

		expected += outputValue
		address, err := ToScriptHash(o.Address)
		if err != nil {
			return nil, errors.New("invalid receiver address")
		}
		tmp := &transaction.TxOutput{
			AssetID:     assetID,
			Value:       outputValue,
			ProgramHash: address,
		}
		output = append(output, tmp)
	}
	log.Debug("expected = %v\n", expected)
	// construct transaction inputs and changes
	coins := wallet.GetCoins()
	sorted := sortAvailableCoinsByValue(coins, account.MultiSign)
	for _, coinItem := range sorted {
		if coinItem.coin.Output.AssetID == assetID && coinItem.coin.Output.ProgramHash == spendAddress {
			input = append(input, coinItem.input)
			log.Debug("coinItem.coin.Output.Value = %v ProgramHash = %x\n", coinItem.coin.Output.Value, spendAddress.ToArrayReverse())
			if coinItem.coin.Output.Value > expected {
				changes := &transaction.TxOutput{
					AssetID:     assetID,
					Value:       coinItem.coin.Output.Value - expected,
					ProgramHash: spendAddress,
				}
				// if any, the changes output of transaction will be the last one
				output = append(output, changes)
				expected = 0
				break
			} else if coinItem.coin.Output.Value == expected {
				expected = 0
				break
			} else if coinItem.coin.Output.Value < expected {
				expected = expected - coinItem.coin.Output.Value
				fmt.Printf("expected - coinItem.coin.Output.Value = %v\n", expected)
			}
		}
	}
	if expected > 0 {
		return nil, errors.New("available token is not enough")
	}

	// construct transaction
	txn, err := transaction.NewTransferAssetTransaction(input, output)
	if err != nil {
		return nil, err
	}
	txAttr := transaction.NewTxAttribute(transaction.Nonce, []byte(strconv.FormatInt(rand.Int63(), 10)))
	txn.Attributes = make([]*transaction.TxAttribute, 0)
	txn.Attributes = append(txn.Attributes, &txAttr)

	ctx := contract.NewContractContext(txn)
	err = wallet.Sign(ctx)
	if err != nil {
		fmt.Println(err)
	}

	if ctx.IsCompleted() {
		txn.SetPrograms(ctx.GetPrograms())
	} else {
		txn.SetPrograms(ctx.GetUncompletedPrograms())
	}

	return txn, nil
}

func sortAvailableCoinsByValue(coins map[*transaction.UTXOTxInput]*account.Coin, addrtype account.AddressType) sortedCoins {
	var coinList sortedCoins
	for in, c := range coins {
		if c.Height <= ledger.DefaultLedger.Blockchain.GetBestHeight() {
			if c.AddressType == addrtype {
				tmp := &sortedCoinsItem{
					input: in,
					coin:  c,
				}
				coinList = append(coinList, tmp)
			}
		}
	}
	sort.Sort(coinList)
	return coinList
}

func deposittransaction(params []interface{}) map[string]interface{} {
	if len(params) < 5 {
		return ElaRpcNil
	}
	var asset, from, address, value, fee, s string
	switch params[0].(type) {
	case string:
		asset = params[0].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[1].(type) {
	case string:
		from = params[1].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[2].(type) {
	case string:
		address = params[2].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[3].(type) {
	case string:
		value = params[3].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[4].(type) {
	case string:
		fee = params[4].(string)
	default:
		return ElaRpcInvalidParameter
	}
	switch params[5].(type) {
	case string:
		s = params[5].(string)
	default:
		return ElaRpcInvalidParameter
	}

	if Wallet == nil {
		return ElaRpc("error : wallet is not opened")
	}

	batchOut := BatchOut{
		Address: address,
		Value:   value,
	}

	tmp, err := HexStringToBytesReverse(asset)
	if err != nil {
		return ElaRpc("error: invalid asset ID")
	}
	var assetID Uint256
	if err := assetID.Deserialize(bytes.NewReader(tmp)); err != nil {
		return ElaRpc("error: invalid asset hash")
	}
	txn, err := MakedepositTransaction(Wallet, assetID, from, fee, s, batchOut)
	if err != nil {
		return ElaRpc("error:" + err.Error())
	}

	var buffer bytes.Buffer
	txn.Serialize(&buffer)
	return ElaRpc(BytesToHexString(buffer.Bytes()))
}

func MakedepositTransaction(wallet account.Client, assetID Uint256, from string, fee string, secret string, batchOut ...BatchOut) (*transaction.Transaction, error) {
	outputNum := len(batchOut)
	if outputNum == 0 {
		return nil, errors.New("nil outputs")
	}

	spendAddress, err := ToScriptHash(from)
	if err != nil {
		return nil, errors.New("invalid sender address")
	}
	var expected Fixed64
	input := []*transaction.UTXOTxInput{}
	output := []*transaction.TxOutput{}
	txnfee, err := StringToFixed64(fee)
	if err != nil || txnfee <= 0 {
		return nil, errors.New("invalid transation fee")
	}
	expected += txnfee
	// construct transaction outputs
	for _, o := range batchOut {
		outputValue, err := StringToFixed64(o.Value)
		if err != nil {
			return nil, err
		}

		expected += outputValue
		address, err := ToScriptHash(o.Address)
		if err != nil {
			return nil, errors.New("invalid receiver address")
		}
		tmp := &transaction.TxOutput{
			AssetID:     assetID,
			Value:       outputValue,
			ProgramHash: address,
		}
		output = append(output, tmp)
	}
	log.Debug("expected = %v\n", expected)
	// construct transaction inputs and changes
	coins := wallet.GetCoins()
	sorted := sortAvailableCoinsByValue(coins, account.MultiSign)
	for _, coinItem := range sorted {
		if coinItem.coin.Output.AssetID == assetID && coinItem.coin.Output.ProgramHash == spendAddress {
			input = append(input, coinItem.input)
			log.Debug("coinItem.coin.Output.Value = %v ProgramHash = %x\n", coinItem.coin.Output.Value, spendAddress.ToArrayReverse())
			if coinItem.coin.Output.Value > expected {
				changes := &transaction.TxOutput{
					AssetID:     assetID,
					Value:       coinItem.coin.Output.Value - expected,
					ProgramHash: spendAddress,
				}
				// if any, the changes output of transaction will be the last one
				output = append(output, changes)
				expected = 0
				break
			} else if coinItem.coin.Output.Value == expected {
				expected = 0
				break
			} else if coinItem.coin.Output.Value < expected {
				expected = expected - coinItem.coin.Output.Value
			}
		}
	}
	if expected > 0 {
		return nil, errors.New("available token is not enough")
	}

	// construct transaction
	txn, err := transaction.NewTransferAssetTransaction(input, output)
	if err != nil {
		return nil, err
	}
	txAttr := transaction.NewTxAttribute(transaction.Nonce, []byte(strconv.FormatInt(rand.Int63(), 10)))
	txn.Attributes = make([]*transaction.TxAttribute, 0)
	txn.Attributes = append(txn.Attributes, &txAttr)

	ctx := contract.NewContractContext(txn)
	err = wallet.Sign(ctx)
	if err != nil {
		fmt.Println(err)
	}

	if ctx.IsCompleted() {
		txn.SetPrograms(ctx.GetPrograms())
	} else {
		txn.SetPrograms(ctx.GetUncompletedPrograms())
	}

	return txn, nil
}
