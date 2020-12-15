//go:generate swagger generate model --accept-definitions-only -f api/swagger.yml -t ./generated
package main

import "github.com/jake-scott/smartthings-nest/cmd"

func main() {
	cmd.Execute()
}
