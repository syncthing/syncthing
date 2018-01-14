// Package cli provides a minimal framework for creating and organizing command line
// Go applications. cli is designed to be easy to understand and write, the most simple
// cli application can be written as follows:
//   func main() {
//     cli.NewApp().Run(os.Args)
//   }
//
// Of course this application does not do much, so let's make this an actual application:
//   func main() {
//     app := cli.NewApp()
//     app.Name = "greet"
//     app.Usage = "say a greeting"
//     app.Action = func(c *cli.Context) error {
//       println("Greetings")
//     }
//
//     app.Run(os.Args)
//   }
package cli

//go:generate python ./generate-flag-types cli -i flag-types.json -o flag_generated.go
