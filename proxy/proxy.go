package proxy

import (
	"fmt"
	"github.com/Vilsol/zappy/models"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

func Serve(endpoints []*models.ProxyEndpoint) {
	wg := sync.WaitGroup{}

	for _, endpoint := range endpoints {
		wg.Add(1)
		temp := endpoint
		go func() {
			proxy(temp)
			wg.Done()
		}()
	}

	wg.Wait()
}

type responseWithError struct {
	Body       []byte
	Header     http.Header
	StatusCode int

	Error error
}

var clientWithoutRedirects = http.Client{
	Transport: nil,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Jar:     nil,
	Timeout: 0,
}

func proxy(endpoint *models.ProxyEndpoint) {
	ttl := time.Duration(viper.GetInt("proxy.ttl")) * time.Minute
	c := cache.New(ttl, ttl*2)

	e := echo.New()

	e.HideBanner = true
	e.HidePort = true

	e.Any("*", func(context echo.Context) error {
		path := context.Request().URL.String()
		targetUrl := endpoint.URL.String() + path
		requestId := context.Response().Header().Get(echo.HeaderXRequestID)

		log.Infof("[%s] %s %s%s -> %s", requestId, context.Request().Method, context.Request().Host, context.Request().URL, targetUrl)

		request, err := http.NewRequest(context.Request().Method, targetUrl, context.Request().Body)

		if err != nil {
			log.Errorf("[%s] Failed constructing URL: %s", requestId, err)
			return err
		}

		for key, val := range context.Request().Header {
			for _, s := range val {
				if key == "Referer" && viper.GetBool("proxy.rewrite-headers") {
					request.Header.Add(key, targetUrl)
				} else if key == "Origin" && viper.GetBool("proxy.rewrite-headers") {
					request.Header.Add(key, endpoint.URL.String())
				} else {
					request.Header.Add(key, s)
				}
			}
		}

		responseChannel := make(chan *responseWithError, 1)

		go func() {
			response, err := clientWithoutRedirects.Do(request)

			if err != nil {
				responseChannel <- &responseWithError{
					Error: err,
				}

				return
			}

			body, err := ioutil.ReadAll(response.Body)

			if err != nil {
				responseChannel <- &responseWithError{
					Error: err,
				}

				return
			}

			responseChannel <- &responseWithError{
				Body:       body,
				Header:     response.Header,
				StatusCode: response.StatusCode,
			}
		}()

		var responseErr *responseWithError

		if request.Method == "GET" {
			timer := time.NewTimer(time.Duration(viper.GetInt("proxy.timeout")) * time.Millisecond)

			select {
			case responseErr = <-responseChannel:
				c.Set(path, responseErr, cache.DefaultExpiration)
				break
			case <-timer.C:
				log.Debugf("[%s] Hit timeout, serving from cache", requestId)
				go StoreCache(c, path, responseChannel)
				first := true
				for {
					val, found := c.Get(path)

					if found {
						responseErr = val.(*responseWithError)
						break
					} else if first {
						first = false
						log.Debugf("[%s] Cache empty, waiting on response", requestId)
					}

					time.Sleep(10 * time.Millisecond)
				}
				break
			}
		} else {
			responseErr = <-responseChannel
		}

		if responseErr.Error != nil {
			log.Errorf("[%s] Failed executing request: %s", requestId, responseErr.Error.Error())
			return responseErr.Error
		}

		for key, val := range responseErr.Header {
			for _, s := range val {
				context.Response().Header().Add(key, s)
			}
		}

		context.Response().WriteHeader(responseErr.StatusCode)

		_, err = context.Response().Write(responseErr.Body)

		if err != nil {
			return err
		}

		return nil
	})

	e.Use(middleware.RequestID())

	log.Infof("Starting proxy to %s on %d", endpoint.URL.String(), endpoint.Port)
	log.Fatal(e.Start(fmt.Sprintf(":%d", endpoint.Port)))
}

func StoreCache(c *cache.Cache, path string, responseChannel chan *responseWithError) {
	responseErr := <-responseChannel
	c.Set(path, responseErr, cache.DefaultExpiration)
}
