package adk

import adkartifact "google.golang.org/adk/v2/artifact"

func newGoogleADKArtifactService() adkartifact.Service {
	return adkartifact.InMemoryService()
}
