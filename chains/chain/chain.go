package chain

import "github.com/blockpilabs/solana-drpc/queue"

var (
	RPC_CHECK_QUEUE_SIZE = 100
)

type Chain struct {

}


func StartRpcCheckQueue() {
	queue.NewQueue(RPC_CHECK_QUEUE_SIZE).Run()
}
