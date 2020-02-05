package core

import (
	"testing"

	//"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mitchellh/devflow/internal/config"
)

func TestNewProject(t *testing.T) {
	require := require.New(t)

	p := TestProject(t,
		WithConfig(config.TestConfig(t, testNewProjectConfig)),
	)

	app, err := p.App("test")
	require.NoError(err)
	require.NotNil(app.Builder)
	require.Nil(app.Registry)
}

const testNewProjectConfig = `
app "test" {
	build "test" {}
}
`
