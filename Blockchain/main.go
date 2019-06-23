package main

import (
	"fmt"
	"os"
	"strconv"

	"./client"
	"./server"

	"github.com/urfave/cli"
)

const (
	PORT = 3141
)

func main() {
	app := cli.NewApp()
	app.Version = "1.0.0 pre-alpha"
	app.Commands = []cli.Command{
		{
			Name:    "client",
			Usage:   "client [upload or check] [path file]",
			Aliases: []string{"client", "c"},
			Action: func(c *cli.Context) error {

				option := c.Args().Get(0)
				nameFile := c.Args().Get(1)

				if option == "1" {
					client.UploadFile("localhost", nameFile)
				} else if option == "2" {
					client.CheckFileExist("localhost", nameFile)
				} else {
					fmt.Println("Option error")
				}

				return nil
			},
		},

		{
			Name:    "server",
			Usage:   "server",
			Aliases: []string{"server", "s"},
			Action: func(c *cli.Context) error {

				server.StartServer(strconv.Itoa(PORT))

				return nil
			},
		},
	}

	app.Run(os.Args)
}
