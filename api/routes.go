package api

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	api.staticRouter.GET("/health", api.healthGET)

	api.staticRouter.POST("/pin", api.pinPOST)
	api.staticRouter.POST("/unpin", api.unpinPOST)

	api.staticRouter.POST("/sweep", api.sweepPOST)
}
