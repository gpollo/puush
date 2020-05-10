package main

import (
	"fmt"
	"os"
	"puush/database"
	"puush/server"
)

func main() {
	db, err := database.ConnectDatabase()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}

	server, err := server.Create(&db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}

	if err = server.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
}
