package events

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/yearn/ydaemon/common/bigNumber"
	"github.com/yearn/ydaemon/common/contracts"
	"github.com/yearn/ydaemon/common/ethereum"
	"github.com/yearn/ydaemon/common/logs"
	"github.com/yearn/ydaemon/internal/models"
	"github.com/yearn/ydaemon/internal/utils"
)

/**************************************************************************************************
** Filter all updateManagementFee events and store them in a map of blockNumber => TEventBlock
**
** Arguments:
** - chainID: the chain ID of the network we are working on
** - vaultAddress: the address of the vault we are working on
** - vaultActivation: the block number at which the vault was activated
** - asyncFeeMap: the async ptr to the map of vaultAddress -> blockNumber -> TEventBlock
** - wg: the async ptr to the WaitGroup to sync the goroutines
**
** Returns nothing as the asyncFeeMap is updated via a pointer
**************************************************************************************************/
func filterUpdateManagementFee(
	chainID uint64,
	vaultAddress common.Address,
	opts *bind.FilterOpts,
	asyncFeeMap *sync.Map,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	client := ethereum.GetRPC(chainID)

	currentVault, _ := contracts.NewYvault043(vaultAddress, client)
	if log, err := currentVault.FilterUpdateManagementFee(opts); err == nil {
		for log.Next() {
			if log.Error() != nil {
				continue
			}

			eventKey := vaultAddress.Hex() + `-` + strconv.FormatUint(uint64(log.Event.Raw.BlockNumber), 10)
			blockData := utils.TEventBlock{
				EventType:   `updateManagementFee`,
				TxHash:      log.Event.Raw.TxHash,
				BlockNumber: log.Event.Raw.BlockNumber,
				TxIndex:     log.Event.Raw.TxIndex,
				LogIndex:    log.Event.Raw.Index,
				Value:       bigNumber.SetInt(log.Event.ManagementFee),
			}

			if syncMap, ok := asyncFeeMap.Load(eventKey); ok {
				currentBlockData := append(syncMap.([]utils.TEventBlock), blockData)
				asyncFeeMap.Store(eventKey, currentBlockData)
			} else {
				asyncFeeMap.Store(eventKey, []utils.TEventBlock{blockData})
			}
		}
	}
}

/**************************************************************************************************
** For each vault we need to know the fee per block, which is the percentage of gains after each
** harvest that will be sent to the governance. This is a dynamic value, and it can be changed
** by the governance. We need to fetch all the events of type `UpdateManagementFee` and build an
** historical mapping of the fee per block, knowing for each block which fee to use.
**
** Arguments:
** - chainID: the chain ID of the network we are working on
** - vaults: the list of vaults we want to fetch the fee for, as a mapping of vaultAddress -> data
** - strategiesList: the list of strategies we want to fetch the fee for, as a mapping of
**   vaultAddress -> strategyAddress -> TStrategyAdded. This one is optional to add initial fee to
**   the strategyPerformanceFee map
**
** Returns:
** - a map of vaultAddress -> blockNumber -> ManagementFee
**************************************************************************************************/
func HandleUpdateManagementFee(
	chainID uint64,
	vaults map[common.Address]*models.TVault,
	start uint64,
	end *uint64,
) map[common.Address]map[uint64][]utils.TEventBlock {
	timeBefore := time.Now()
	asyncManagementFeeUpdate := sync.Map{}

	wg := &sync.WaitGroup{}
	for _, v := range vaults {
		wg.Add(1)
		opts := &bind.FilterOpts{Start: start, End: end}
		if start == 0 {
			opts = &bind.FilterOpts{Start: v.Activation, End: end}
		}

		go filterUpdateManagementFee(chainID, v.Address, opts, &asyncManagementFeeUpdate, wg)
	}
	wg.Wait()

	/**********************************************************************************************
	** Once all vaults ManagementFees updates have been retrieved, we need to extract them from the
	** sync.Map.
	**
	** The syncMap variable is setup as follows:
	** - key: vaultAddress-blockNumber
	** - value: []TEventBlock
	**
	** We need to transform it into a map as follows:
	** - vaultAddress -> blockNumber -> []TEventBlock
	**********************************************************************************************/
	managementFeeForVaults := make(map[common.Address]map[uint64][]utils.TEventBlock)
	asyncManagementFeeUpdate.Range(func(key, value interface{}) bool {
		eventKey := strings.Split(key.(string), `-`)
		vaultAddress := common.HexToAddress(eventKey[0])
		blockNumber, _ := strconv.ParseUint(eventKey[1], 10, 64)

		if _, ok := managementFeeForVaults[vaultAddress]; !ok {
			managementFeeForVaults[vaultAddress] = make(map[uint64][]utils.TEventBlock)
		}
		managementFeeForVaults[vaultAddress][blockNumber] = value.([]utils.TEventBlock)
		return true
	})

	logs.Success(`It tooks`, time.Since(timeBefore), `to retrieve the managementFee updates`)
	return managementFeeForVaults
}