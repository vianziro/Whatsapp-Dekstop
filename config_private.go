package main

// Private build configuration & environment security.
// IMPORTANT: This file is intentionally kept local and MUST NOT be pushed to git repository.
// Required to compile and run the application.

const (
	windowTitle     = "WhatsApp Desk"
	appURL          = "https://web.whatsapp.com"
	appBuildChannel = "production-release"
	securitySecret  = "WADL_9F8EE738E2B1_SECURE_BUILD"
)

func validateBuildEnvironment() bool {
	return appBuildChannel == "production-release" && len(securitySecret) > 16
}
