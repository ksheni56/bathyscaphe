package indexer

import (
	"fmt"
	configapi "github.com/creekorful/trandoshan/internal/configapi/client"
	"github.com/creekorful/trandoshan/internal/constraint"
	"github.com/creekorful/trandoshan/internal/event"
	"github.com/creekorful/trandoshan/internal/indexer/index"
	"github.com/creekorful/trandoshan/internal/process"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"net/http"
)

var errHostnameNotAllowed = fmt.Errorf("hostname is not allowed")

// State represent the application state
type State struct {
	index        index.Index
	indexDriver  string
	configClient configapi.Client
}

// Name return the process name
func (state *State) Name() string {
	return "indexer"
}

// CommonFlags return process common flags
func (state *State) CommonFlags() []string {
	return []string{process.HubURIFlag, process.ConfigAPIURIFlag}
}

// CustomFlags return process custom flags
func (state *State) CustomFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "index-driver",
			Usage:    "Name of the storage driver",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "index-dest",
			Usage:    "Destination (config) passed to the driver",
			Required: true,
		},
	}
}

// Initialize the process
func (state *State) Initialize(provider process.Provider) error {
	indexDriver := provider.GetValue("index-driver")
	idx, err := index.NewIndex(indexDriver, provider.GetValue("index-dest"))
	if err != nil {
		return err
	}
	state.index = idx
	state.indexDriver = indexDriver

	configClient, err := provider.ConfigClient([]string{configapi.ForbiddenHostnamesKey})
	if err != nil {
		return err
	}
	state.configClient = configClient

	return nil
}

// Subscribers return the process subscribers
func (state *State) Subscribers() []process.SubscriberDef {
	return []process.SubscriberDef{
		{Exchange: event.NewResourceExchange, Queue: fmt.Sprintf("%sIndexingQueue", state.indexDriver), Handler: state.handleNewResourceEvent},
	}
}

// HTTPHandler returns the HTTP API the process expose
func (state *State) HTTPHandler() http.Handler {
	return nil
}

func (state *State) handleNewResourceEvent(subscriber event.Subscriber, msg event.RawMessage) error {
	var evt event.NewResourceEvent
	if err := subscriber.Read(&msg, &evt); err != nil {
		return err
	}

	// make sure hostname hasn't been flagged as forbidden
	if allowed, err := constraint.CheckHostnameAllowed(state.configClient, evt.URL); !allowed || err != nil {
		return fmt.Errorf("%s %w", evt.URL, errHostnameNotAllowed)
	}

	if err := state.index.IndexResource(evt.URL, evt.Time, evt.Body, evt.Headers); err != nil {
		return fmt.Errorf("error while indexing resource: %s", err)
	}

	log.Info().Str("url", evt.URL).Msg("Successfully indexed resource")

	return nil
}
