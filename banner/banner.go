package banner

import (
	"fmt"
)

// prints the version message
const version = "v0.0.2"

func PrintVersion() {
	fmt.Printf("Current waybackurlsx version %s\n", version)
}

// Prints the Colorful banner
func PrintBanner() {
	banner := `
                          __                  __                 __           
 _      __ ____ _ __  __ / /_   ____ _ _____ / /__ __  __ _____ / /_____ _  __
| | /| / // __  // / / // __ \ / __  // ___// //_// / / // ___// // ___/| |/_/
| |/ |/ // /_/ // /_/ // /_/ // /_/ // /__ / ,<  / /_/ // /   / /(__  )_>  <  
|__/|__/ \__,_/ \__, //_.___/ \__,_/ \___//_/|_| \__,_//_/   /_//____//_/|_|  
               /____/
`
	fmt.Printf("%s\n%75s\n\n", banner, "Current waybackurlsx version "+version)
}
