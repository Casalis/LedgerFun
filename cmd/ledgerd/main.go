package main

import (
	"fmt"

	"github.com/Casalis/LedgerFun/internal/store"
	"github.com/gofor-little/env"
)

func main() {

	if err := env.Load(".env2"); err != nil {
		panic(err)
	}

	// var conn pgx.Conn
	// var err error
	conn, err := store.Connect()

	if err != nil {
		fmt.Printf("failed to connect to DB : %d", err)
		return
	}

	conn.Close()
}
