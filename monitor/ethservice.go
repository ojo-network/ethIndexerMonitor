package monitor

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	gql "github.com/hasura/go-graphql-client"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/ojo-network/ethIndexerMonitor/config"
)

type (
	Pool struct {
		ID          string `graphql:"id"`
		Token0Price string `graphql:"token0Price"`
		Token1Price string `graphql:"token1Price"`
		Token0      struct {
			ID string `graphql:"id"`
		} `json:"token0"`
		Token1 struct {
			ID string `graphql:"id"`
		} `json:"token1"`
	}

	Data struct {
		Pools []Pool `graphql:"pools(where:{id_in: $ids})"`
	}

	ethChecker struct {
		logger       zerolog.Logger
		network      string
		truthUrl     *gql.Client
		checkUrl     *gql.Client
		deviation    float64
		cronDuration time.Duration

		mut sync.Mutex

		//id to map detail
		pools map[string]*PoolDetail

		//asset name to id

		assetIdMap map[string]string
	}

	PoolDetail struct {
		config.Pool
		truePrice     float64
		expectedPrice float64
	}

	EthService struct {
		services map[string]*ethChecker
	}
)

func StartEthServices(ctx context.Context, logger zerolog.Logger, config config.Config) *EthService {
	eLogger := logger.With().Str("module", "eth-service").Logger()
	es := EthService{
		services: make(map[string]*ethChecker),
	}

	for network, details := range config.NetworkMap {
		cronDuration, _ := time.ParseDuration(details.CronInterval)
		service := newEthMonitorService(ctx, eLogger, details.Deviation, network, details.TruthUrl, details.CheckUrl, cronDuration, details.Pools)
		es.services[network] = service

		eLogger.Info().Str("network", network).
			Str("check_url", details.CheckUrl).
			Str("truth_url", details.TruthUrl).
			Msg("monitoring")
	}

	return &es
}

func newEthMonitorService(ctx context.Context, logger zerolog.Logger, deviation float64, network, truthUrl, checkUrl string, cronDuration time.Duration, pools []config.Pool) *ethChecker {
	poolIndex := make(map[string]*PoolDetail)
	assetIdMap := make(map[string]string)
	for _, pool := range pools {
		pool.TokenAddress = strings.ToLower(pool.TokenAddress)
		poolIndex[strings.ToLower(pool.Address)] = &PoolDetail{pool, 0, 0}
		assetIdMap[strings.ToLower(pool.TokenName)] = pool.Address
	}

	service := &ethChecker{
		logger:       logger.With().Str("network", network).Logger(),
		deviation:    deviation,
		network:      network,
		cronDuration: cronDuration,
		truthUrl:     gql.NewClient(truthUrl, nil),
		checkUrl:     gql.NewClient(checkUrl, nil),
		pools:        poolIndex,
		assetIdMap:   assetIdMap,
	}

	go service.startCron(ctx, pools)

	return service
}

func (es *ethChecker) startCron(ctx context.Context, pools []config.Pool) {
	ids := make([]string, len(pools))
	for i, pool := range pools {
		ids[i] = pool.Address
	}

	for {
		select {
		case <-ctx.Done():
			wg.Done()
			return

		default:
			err := es.checkAssetPrice(ctx, ids)
			if err != nil {
				es.logger.Err(err).
					Str("network", es.network).
					Msg("Error in querying request ids")
			}

			time.Sleep(es.cronDuration)
		}
	}
}

func (es *ethChecker) checkAssetPrice(ctx context.Context, ids []string) error {
	g, _ := errgroup.WithContext(ctx)
	var (
		truth Data
		check Data
	)

	g.Go(func() error {
		ids := ids
		priceData, err := GetPrices(es.truthUrl, ids)
		if err != nil {
			return err
		}

		truth = priceData

		return nil
	})

	g.Go(func() error {
		priceData, err := GetPrices(es.checkUrl, ids)
		if err != nil {
			return err
		}

		check = priceData

		return nil
	})

	err := g.Wait()
	if err != nil {
		return err
	}

	truthIndex := make(map[string]Pool)
	for _, truth := range truth.Pools {
		truthIndex[truth.ID] = truth
	}
	for _, check := range check.Pools {
		if truth, found := truthIndex[check.ID]; !found {
			// asset not found error
			es.logger.Err(fmt.Errorf("%s asset not found", check.ID)).Send()
			continue
		} else {
			var truePrice, expectedPrice float64
			var tokenName = es.pools[check.ID].TokenName

			switch es.pools[check.ID].TokenAddress {
			// quote for a specific base
			case check.Token0.ID:
				truePrice, err = strconv.ParseFloat(truth.Token1Price, 64)
				if err != nil {
					return err
				}

				expectedPrice, err = strconv.ParseFloat(check.Token1Price, 64)
				if err != nil {
					return err
				}

			case check.Token1.ID:
				truePrice, err = strconv.ParseFloat(truth.Token0Price, 64)
				if err != nil {
					return err
				}

				expectedPrice, err = strconv.ParseFloat(check.Token0Price, 64)
				if err != nil {
					return err
				}
			}

			if truePrice != expectedPrice {
				if (math.Abs(truePrice-expectedPrice)/truePrice)*100 > es.deviation {
					slackChan <- createMismatchedDeviationAttachment(
						truePrice,
						expectedPrice,
						es.network,
						tokenName,
					)
				}
			}

			pool := es.pools[check.ID]
			pool.expectedPrice = expectedPrice
			pool.truePrice = truePrice
		}
	}

	return nil
}

func GetPrices(c *gql.Client, ids []string) (Data, error) {
	var bundle Data
	err := c.Query(context.Background(), &bundle, map[string]interface{}{
		"ids": ids,
	})

	if err != nil {
		return Data{}, err
	}

	return bundle, nil
}

func (ec *EthService) getPrices(network, assetName string) (float64, float64, error) {
	service, found := ec.services[network]
	if !found {
		return 0, 0, fmt.Errorf("network %s not found", network)
	}

	service.mut.Lock()
	defer service.mut.Unlock()
	id, found := service.assetIdMap[assetName]
	if !found {
		return 0, 0, fmt.Errorf("asset %s not found on network %s", assetName, network)
	}

	pool, found := service.pools[id]
	if !found {
		return 0, 0, fmt.Errorf("asset %s not synced on indexer %s", assetName, network)
	}

	return pool.truePrice, pool.expectedPrice, nil
}
