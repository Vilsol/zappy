package cmd

import (
	"fmt"
	"github.com/Vilsol/zappy/models"
	"github.com/Vilsol/zappy/proxy"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"net/url"
	"strconv"
)

func init() {
	proxyCmd.Flags().IntP("timeout", "t", 1000, "Timeout in milliseconds until request should fallback to cache")
	proxyCmd.Flags().Int("ttl", 60, "TTL in minutes for cache")
	proxyCmd.Flags().Bool("rewrite-headers", true, "Rewrite Origin and Referer headers to current request")

	_ = viper.BindPFlag("proxy.timeout", proxyCmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("proxy.ttl", proxyCmd.Flags().Lookup("ttl"))
	_ = viper.BindPFlag("proxy.rewrite-headers", proxyCmd.Flags().Lookup("rewrite-headers"))

	rootCmd.AddCommand(proxyCmd)
}

var proxyCmd = &cobra.Command{
	Use: "proxy [flags] <url> <port> ...",
	Example: "Serving a single url on port 8080:" +
		"\nzappy proxy https://baconipsum.com 8080" +
		"\n" +
		"\nServing multiple urls on ports 8080 and 8081:" +
		"\nzappy proxy https://baconipsum.com 8080 https://loripsum.net 8081",
	Short: "Proxy the provided urls to the ports",
	Long: "Proxy the provided urls to the ports. You can provide as many urls and ports in order as you want." +
		"\nNote: Do not add a trailing slash!",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("requires at least %d arg(s), only received %d", 2, len(args))
		}

		if len(args)%2 != 0 {
			return fmt.Errorf("requires an even number of arguments, received %d", len(args))
		}

		for i := 0; i < len(args); i++ {
			if i%2 == 0 {
				_, err := url.ParseRequestURI(args[i])
				if err != nil {
					return fmt.Errorf("%s is not a valid url", args[i])
				}
			} else {
				port, err := strconv.Atoi(args[i])
				if err != nil {
					return fmt.Errorf("%s is not a valid port", args[i])
				}

				if port > 65535 {
					return fmt.Errorf("port %d is higher than 65535", port)
				}
			}
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		endpoints := make([]*models.ProxyEndpoint, len(args)/2)

		for i := 0; i < len(args); i += 2 {
			url, _ := url.ParseRequestURI(args[i])
			port, _ := strconv.Atoi(args[i+1])
			endpoints[i/2] = &models.ProxyEndpoint{
				URL:  url,
				Port: port,
			}
		}

		proxy.Serve(endpoints)
	},
}
