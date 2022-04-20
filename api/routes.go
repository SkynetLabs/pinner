package api

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	api.staticRouter.GET("/health", api.healthGET)

	api.staticRouter.POST("/pin", api.pinPOST)
	api.staticRouter.POST("/unpin", api.unpinPOST)

	// TODO This is a placeholder for an endpoint which will announce a server
	//  as dead and will remove it as pinner from all skylinks.
	api.staticRouter.POST("/deadserver", func(_ http.ResponseWriter, _ *http.Request, _ httprouter.Params) {})

	api.staticRouter.POST("/sweep", api.sweepPOST)
}
