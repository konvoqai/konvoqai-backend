package main

import (
	"log"

	"golan-project/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
