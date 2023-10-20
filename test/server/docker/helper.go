package docker

import (
	"context"
	"errors"
	"github.com/testcontainers/testcontainers-go/wait"
	"log"
	"net"
	"time"
)

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitretry(timeout time.Duration, f func(ctx context.Context, target wait.StrategyTarget) error) func(ctx context.Context, target wait.StrategyTarget) error {
	return func(ctx context.Context, target wait.StrategyTarget) (err error) {
		return retry(timeout, func() error {
			return f(ctx, target)
		})
	}
}

func retry(timeout time.Duration, f func() error) (err error) {
	err = errors.New("initial")
	start := time.Now()
	for i := int64(1); err != nil && time.Since(start) < timeout; i++ {
		err = f()
		if err != nil {
			log.Println("ERROR: :", err)
			wait := time.Duration(i) * time.Second
			if time.Since(start)+wait < timeout {
				log.Println("ERROR: retry after:", wait.String())
				time.Sleep(wait)
			} else {
				time.Sleep(time.Since(start) + wait - timeout)
				return f()
			}
		}
	}
	return err
}
