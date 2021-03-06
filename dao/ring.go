/*

  Copyright 2017 Loopring Project Ltd (Loopring Foundation).

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.

*/

package dao

import (
	"github.com/Loopring/relay/types"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
)

type Ring struct {
	ID   int    `gorm:"column:id;primary_key;"`
	Hash string `gorm:"column:hash;type:varchar(82)"`
}

type FilledOrder struct {
	ID               int    `gorm:"column:id;primary_key;"`
	RingHash         string `gorm:"column:ringhash;type:varchar(82)"`
	OrderHash        string `gorm:"column:orderhash;type:varchar(82)"`
	FeeSelection     uint8  `gorm:"column:fee_selection" json:"feeSelection"`
	RateAmountS      string `gorm:"column:rate_amount_s;type:varchar(82)" json:"rateAmountS"`
	AvailableAmountS string `gorm:"column:available_amount_s;type:varchar(82)"json:"availableAmountS"`
	AvailableAmountB string `gorm:"column:available_amount_b;type:varchar(82)"`
	FillAmountS      string `gorm:"column:fill_amount_s;type:varchar(82)" json:"fillAmountS"`
	FillAmountB      string `gorm:"column:fill_amount_b;type:varchar(82)" json:"fillAmountB"`
	LrcReward        string `gorm:"column:lrc_reward;type:varchar(82)" json:"lrcReward"`
	LrcFee           string `gorm:"column:lrc_fee;type:varchar(82)" json:"lrcFee"`
	FeeS             string `gorm:"column:fee_s;type:varchar(82)" json:"feeS"`
	LegalFee         string `gorm:"column:legal_fee;type:varchar(82)" json:"legalFee"`
	SPrice           string `gorm:"column:s_price;type:varchar(82)" json:"sPrice"`
	BPrice           string `gorm:"column:b_price;type:varchar(82)" json:"sPrice"`
}

func getRatString(v *big.Rat) string {
	if nil == v {
		return ""
	} else {
		return v.String()
	}
}

func (daoFilledOrder *FilledOrder) ConvertDown(filledOrder *types.FilledOrder, ringhash common.Hash) error {
	daoFilledOrder.RingHash = ringhash.Hex()
	daoFilledOrder.OrderHash = filledOrder.OrderState.RawOrder.Hash.Hex()
	daoFilledOrder.FeeSelection = filledOrder.FeeSelection
	daoFilledOrder.RateAmountS = getRatString(filledOrder.RateAmountS)
	daoFilledOrder.AvailableAmountS = getRatString(filledOrder.AvailableAmountS)
	daoFilledOrder.AvailableAmountB = getRatString(filledOrder.AvailableAmountB)
	daoFilledOrder.FillAmountS = getRatString(filledOrder.FillAmountS)
	daoFilledOrder.FillAmountB = getRatString(filledOrder.FillAmountB)
	daoFilledOrder.LrcReward = getRatString(filledOrder.LrcReward)
	daoFilledOrder.LrcFee = getRatString(filledOrder.LrcFee)
	daoFilledOrder.FeeS = getRatString(filledOrder.FeeS)
	daoFilledOrder.LegalFee = getRatString(filledOrder.LegalFee)
	daoFilledOrder.SPrice = getRatString(filledOrder.SPrice)
	daoFilledOrder.BPrice = getRatString(filledOrder.BPrice)
	return nil
}

func (daoFilledOrder *FilledOrder) ConvertUp(filledOrder *types.FilledOrder, rds RdsService) error {
	if nil != rds {
		daoOrderState, err := rds.GetOrderByHash(common.HexToHash(daoFilledOrder.OrderHash))
		if nil != err {
			return err
		}
		orderState := &types.OrderState{}
		daoOrderState.ConvertUp(orderState)
		filledOrder.OrderState = *orderState
	}
	filledOrder.FeeSelection = daoFilledOrder.FeeSelection
	filledOrder.RateAmountS = new(big.Rat)
	filledOrder.RateAmountS.SetString(daoFilledOrder.RateAmountS)
	filledOrder.AvailableAmountS = new(big.Rat)
	filledOrder.AvailableAmountB = new(big.Rat)
	filledOrder.AvailableAmountS.SetString(daoFilledOrder.AvailableAmountS)
	filledOrder.AvailableAmountB.SetString(daoFilledOrder.AvailableAmountB)
	filledOrder.FillAmountS = new(big.Rat)
	filledOrder.FillAmountB = new(big.Rat)
	filledOrder.FillAmountS.SetString(daoFilledOrder.FillAmountS)
	filledOrder.FillAmountB.SetString(daoFilledOrder.FillAmountB)
	filledOrder.LrcReward = new(big.Rat)
	filledOrder.LrcFee = new(big.Rat)
	filledOrder.LrcReward.SetString(daoFilledOrder.LrcReward)
	filledOrder.LrcFee.SetString(daoFilledOrder.LrcFee)
	filledOrder.FeeS = new(big.Rat)
	filledOrder.FeeS.SetString(daoFilledOrder.FeeS)
	filledOrder.LegalFee = new(big.Rat)
	filledOrder.LegalFee.SetString(daoFilledOrder.LegalFee)
	filledOrder.SPrice = new(big.Rat)
	filledOrder.SPrice.SetString(daoFilledOrder.SPrice)
	filledOrder.BPrice = new(big.Rat)
	filledOrder.BPrice.SetString(daoFilledOrder.BPrice)
	return nil
}

type RingSubmitInfo struct {
	ID               int    `gorm:"column:id;primary_key;"`
	RingHash         string `gorm:"column:ringhash;type:varchar(82)"`
	ProtocolAddress  string `gorm:"column:protocol_address;type:varchar(42)"`
	OrdersCount      int64  `gorm:"column:order_count;type:bigint"`
	ProtocolData     string `gorm:"column:protocol_data;type:text"`
	ProtocolGas      string `gorm:"column:protocol_gas;type:varchar(50)"`
	ProtocolGasPrice string `gorm:"column:protocol_gas_price;type:varchar(50)"`
	ProtocolUsedGas  string `gorm:"column:protocol_used_gas;type:varchar(50)"`

	RegistryData     string `gorm:"column:registry_data;type:text"`
	RegistryGas      string `gorm:"column:registry_gas;type:varchar(50)"`
	RegistryGasPrice string `gorm:"column:registry_gas_price;type:varchar(50)"`
	RegistryUsedGas  string `gorm:"column:registry_used_gas;type:varchar(50)"`

	ProtocolTxHash string `gorm:"column:protocol_tx_hash;type:varchar(82)"`
	RegistryTxHash string `gorm:"column:registry_tx_hash;type:varchar(82)"`

	Miner string `gorm:"column:miner;type:varchar(42)"`
	Err   string `gorm:"column:err;type:text"`
}

func getBigIntString(v *big.Int) string {
	if nil == v {
		return ""
	} else {
		return v.String()
	}
}

func (info *RingSubmitInfo) ConvertDown(typesInfo *types.RingSubmitInfo) error {
	info.RingHash = typesInfo.Ringhash.Hex()
	info.ProtocolAddress = typesInfo.ProtocolAddress.Hex()
	info.OrdersCount = typesInfo.OrdersCount.Int64()
	info.ProtocolData = common.ToHex(typesInfo.ProtocolData)
	info.ProtocolGas = getBigIntString(typesInfo.ProtocolGas)
	info.ProtocolUsedGas = getBigIntString(typesInfo.ProtocolUsedGas)
	info.ProtocolGasPrice = getBigIntString(typesInfo.ProtocolGasPrice)
	info.RegistryData = common.ToHex(typesInfo.RegistryData)
	info.RegistryGas = getBigIntString(typesInfo.RegistryGas)
	info.RegistryUsedGas = getBigIntString(typesInfo.RegistryUsedGas)
	info.RegistryGasPrice = getBigIntString(typesInfo.RegistryGasPrice)
	info.Miner = typesInfo.Miner.Hex()
	return nil
}

func (info *RingSubmitInfo) ConvertUp(typesInfo *types.RingSubmitInfo) error {
	typesInfo.Ringhash = common.HexToHash(info.RingHash)
	typesInfo.ProtocolAddress = common.HexToAddress(info.ProtocolAddress)
	typesInfo.OrdersCount = big.NewInt(info.OrdersCount)
	typesInfo.ProtocolData = common.FromHex(info.ProtocolData)
	typesInfo.ProtocolGas = new(big.Int)
	typesInfo.ProtocolGas.SetString(info.ProtocolGas, 0)
	typesInfo.ProtocolUsedGas = new(big.Int)
	typesInfo.ProtocolUsedGas.SetString(info.ProtocolUsedGas, 0)
	typesInfo.ProtocolGasPrice = new(big.Int)
	typesInfo.ProtocolGasPrice.SetString(info.ProtocolGasPrice, 0)
	typesInfo.RegistryData = common.FromHex(info.RegistryData)
	typesInfo.RegistryGas = new(big.Int)
	typesInfo.RegistryGas.SetString(info.RegistryGas, 0)
	typesInfo.RegistryUsedGas = new(big.Int)
	typesInfo.RegistryUsedGas.SetString(info.RegistryUsedGas, 0)
	typesInfo.RegistryGasPrice = new(big.Int)
	typesInfo.RegistryGasPrice.SetString(info.RegistryGasPrice, 0)
	typesInfo.SubmitTxHash = common.HexToHash(info.ProtocolTxHash)
	typesInfo.RegistryTxHash = common.HexToHash(info.RegistryTxHash)
	typesInfo.Miner = common.HexToAddress(info.Miner)
	return nil
}

func (s *RdsServiceImpl) UpdateRingSubmitInfoRegistryTxHash(ringhashs []common.Hash, txHash string) error {
	hashes := []string{}
	for _, h := range ringhashs {
		hashes = append(hashes, h.Hex())
	}
	dbForUpdate := s.db.Model(&RingSubmitInfo{}).Where("ringhash in (?)", hashes)
	return dbForUpdate.Update("registry_tx_hash", txHash).Error
}

func (s *RdsServiceImpl) UpdateRingSubmitInfoFailed(ringhashs []common.Hash, err string) error {
	hashes := []string{}
	for _, h := range ringhashs {
		hashes = append(hashes, h.Hex())
	}
	dbForUpdate := s.db.Model(&RingSubmitInfo{}).Where("ringhash in (?) ", hashes)
	return dbForUpdate.Update("err", err).Error
}

func (s *RdsServiceImpl) UpdateRingSubmitInfoProtocolTxHash(ringhash common.Hash, txHash string) error {
	dbForUpdate := s.db.Model(&RingSubmitInfo{}).Where("ringhash = ?", ringhash.Hex())
	return dbForUpdate.Update("protocol_tx_hash", txHash).Error
}

func (s *RdsServiceImpl) GetRingForSubmitByHash(ringhash common.Hash) (ringForSubmit RingSubmitInfo, err error) {
	err = s.db.Where("ringhash = ? ", ringhash.Hex()).First(&ringForSubmit).Error
	return
}

func (s *RdsServiceImpl) GetRingHashesByTxHash(txHash common.Hash) ([]common.Hash, error) {
	var (
		err       error
		hashes    []common.Hash
		hashesStr []string
	)

	err = s.db.Model(&RingSubmitInfo{}).Where("registry_tx_hash = ? or submit_tx_hash = ? ", txHash.Hex(), txHash.Hex()).Pluck("ringhash", hashesStr).Error
	for _, h := range hashesStr {
		hashes = append(hashes, common.HexToHash(h))
	}
	return hashes, err
}

func (s *RdsServiceImpl) UpdateRingSubmitInfoRegistryUsedGas(txHash string, usedGas *big.Int) error {
	dbForUpdate := s.db.Model(&RingSubmitInfo{}).Where("registry_tx_hash = ?", txHash)
	return dbForUpdate.Update("registry_used_gas", getBigIntString(usedGas)).Error
}

func (s *RdsServiceImpl) UpdateRingSubmitInfoSubmitUsedGas(txHash string, usedGas *big.Int) error {
	dbForUpdate := s.db.Model(&RingSubmitInfo{}).Where("protocol_tx_hash = ?", txHash)
	return dbForUpdate.Update("protocol_used_gas", getBigIntString(usedGas)).Error
}
