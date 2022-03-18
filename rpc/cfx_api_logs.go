package rpc

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/conflux-chain/conflux-infura/node"
	"github.com/conflux-chain/conflux-infura/rpc/throttle"
	"github.com/conflux-chain/conflux-infura/store/mysql"
	"github.com/conflux-chain/conflux-infura/util"
	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const thresholdGetLogs = 1

var errQuotaNotEnough = errors.New("quota not enough")

type CfxLogApi struct {
	pool       *node.CfxClientProvider
	store      *mysql.UserStore
	throttling *throttle.RefCounter
}

func NewCfxLogApi(pool *node.CfxClientProvider, store *mysql.UserStore, client *redis.Client) *CfxLogApi {
	return &CfxLogApi{
		pool:       pool,
		store:      store,
		throttling: throttle.NewRefCounter(client, thresholdGetLogs),
	}
}

func (api *CfxLogApi) GetLogs(ctx context.Context, filter types.LogFilter) ([]types.Log, error) {
	logs, ok, err := api.getLogsByUser(ctx, filter)
	if err != nil {
		return emptyLogs, err
	}

	if ok {
		return logs, nil
	}

	client, err := api.pool.GetClientByIPGroup(ctx, node.GroupCfxArchives)
	if err == node.ErrClientUnavailable {
		return nil, errQuotaNotEnough
	}

	if err != nil {
		return nil, err
	}

	return api.getLogsThrottled(client, filter)
}

func (api *CfxLogApi) getLogsByUser(ctx context.Context, filter types.LogFilter) ([]types.Log, bool, error) {
	request, ok := ctx.Value("request").(*http.Request)
	if !ok {
		logrus.Error("HTTP request instance not found in RPC context")
		return nil, false, nil
	}

	if request.URL == nil {
		logrus.Error("URL not set in HTTP request")
		return nil, false, nil
	}

	key := strings.TrimLeft(request.URL.Path, "/")
	if idx := strings.Index(key, "/"); idx > 0 {
		key = key[:idx]
	}

	user, ok, err := api.store.GetUserByKey(key)
	if err != nil {
		logrus.WithError(err).WithField("key", key).Warn("Failed to get user by key")
		return nil, false, err
	}

	if !ok {
		return nil, false, nil
	}

	// TODO cache client for user
	client, err := sdk.NewClient(user.NodeUrl)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"user": user.Name,
			"node": user.NodeUrl,
		}).Warn("Failed to connect to full node for user")
		return nil, false, err
	}
	defer client.Close()

	logs, err := api.getLogsThrottled(client, filter)
	if err != nil {
		return nil, false, err
	}

	return logs, true, nil
}

func (api *CfxLogApi) getLogsThrottled(cfx sdk.ClientOperator, filter types.LogFilter) ([]types.Log, error) {
	nodeName := util.Url2NodeName(cfx.GetNodeURL())
	key := fmt.Sprintf("rpc:throttle:cfx_getLogs:%v", nodeName)
	if !api.throttling.Ref(key) {
		return nil, errQuotaNotEnough
	}
	defer api.throttling.UnrefAsync(key)

	return cfx.GetLogs(filter)
}
