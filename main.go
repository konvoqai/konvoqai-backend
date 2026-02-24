package main

import (
	"log"

	"konvoq-backend/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
