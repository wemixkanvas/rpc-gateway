module github.com/conflux-chain/conflux-infura

go 1.15

require (
	github.com/Conflux-Chain/go-conflux-sdk v1.0.10-0.20210609053701-d7a410d320fb
	github.com/buraksezer/consistent v0.9.0
	github.com/cespare/xxhash v1.1.0
	github.com/ethereum/go-ethereum v1.9.25
	github.com/go-redis/redis/v8 v8.8.2
	github.com/nsf/jsondiff v0.0.0-20210303162244-6ea32392771e
	github.com/pkg/errors v0.9.1
	github.com/royeo/dingrobot v1.0.1-0.20191230075228-c90a788ca8fd
	github.com/selvatico/go-mocket v1.0.7
	github.com/sirupsen/logrus v1.8.0
	github.com/spf13/cobra v1.1.3
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.7.0
	gorm.io/driver/mysql v1.0.5
	gorm.io/gorm v1.21.3
)

// replace github.com/Conflux-Chain/go-conflux-sdk => ../go-conflux-sdk
