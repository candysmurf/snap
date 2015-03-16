package control

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/intelsdilabs/gomit"

	"github.com/intelsdilabs/pulse/control/plugin"
	"github.com/intelsdilabs/pulse/control/plugin/cpolicy"
	"github.com/intelsdilabs/pulse/core"
	"github.com/intelsdilabs/pulse/core/cdata"
	"github.com/intelsdilabs/pulse/core/control_event"
)

// control private key (RSA private key)
// control public key (RSA public key)
// Plugin token = token generated by plugin and passed to control
// Session token = plugin seed encrypted by control private key, verified by plugin using control public key
//

type executablePlugins []plugin.ExecutablePlugin

type pluginControl struct {
	// TODO, going to need coordination on changing of these
	RunningPlugins executablePlugins
	Started        bool
	// loadRequestsChan chan LoadedPlugin

	controlPrivKey *rsa.PrivateKey
	controlPubKey  *rsa.PublicKey
	eventManager   *gomit.EventController

	pluginManager managesPlugins
	metricCatalog catalogsMetrics
	pluginRunner  runsPlugins
	pluginRouter  routesToPlugins
}

type routesToPlugins interface {
	Collect([]core.MetricType, *cdata.ConfigDataNode, time.Time) (*collectionResponse, error)
	SetRunner(runsPlugins)
	SetMetricCatalog(catalogsMetrics)
}

type runsPlugins interface {
	Start() error
	Stop() []error
	AvailablePlugins() *availablePlugins
	AddDelegates(delegates ...gomit.Delegator)
	SetMetricCatalog(c catalogsMetrics)
	SetPluginManager(m managesPlugins)
}

type managesPlugins interface {
	LoadPlugin(string) (*loadedPlugin, error)
	UnloadPlugin(CatalogedPlugin) error
	LoadedPlugins() *loadedPlugins
	SetMetricCatalog(catalogsMetrics)
	GenerateArgs() plugin.Arg
}

type catalogsMetrics interface {
	Get([]string, int) (*metricType, error)
	Add(*metricType)
	AddLoadedMetricType(*loadedPlugin, core.MetricType)
	Item() (string, []*metricType)
	Next() bool
	Subscribe([]string, int) error
	Unsubscribe([]string, int) error
	Table() map[string][]*metricType
	GetPlugin([]string, int) (*loadedPlugin, error)
}

// Returns a new pluginControl instance
func New() *pluginControl {
	c := &pluginControl{}
	// Initialize components
	//
	// Event Manager
	c.eventManager = gomit.NewEventController()

	// Metric Catalog
	c.metricCatalog = newMetricCatalog()

	// Plugin Manager
	c.pluginManager = newPluginManager()
	//    Plugin Manager needs a reference to the metric catalog
	c.pluginManager.SetMetricCatalog(c.metricCatalog)

	// Plugin Runner
	c.pluginRunner = newRunner()
	c.pluginRunner.AddDelegates(c.eventManager)
	c.pluginRunner.SetMetricCatalog(c.metricCatalog)
	c.pluginRunner.SetPluginManager(c.pluginManager)

	// Plugin Router
	c.pluginRouter = newPluginRouter()
	c.pluginRouter.SetRunner(c.pluginRunner)
	c.pluginRouter.SetMetricCatalog(c.metricCatalog)

	// Wire event manager

	// Start stuff
	err := c.pluginRunner.Start()
	if err != nil {
		panic(err)
	}

	// c.loadRequestsChan = make(chan LoadedPlugin)
	// privatekey, err := rsa.GenerateKey(rand.Reader, 4096)

	// if err != nil {
	// 	panic(err)
	// }

	// // Future use for securing.
	// c.controlPrivKey = privatekey
	// c.controlPubKey = &privatekey.PublicKey

	return c
}

// Begin handling load, unload, and inventory
func (p *pluginControl) Start() error {
	// begin controlling

	// Start load handler. We only start one to keep load requests handled in
	// a linear fashion for now as this is a low priority.
	// go p.HandleLoadRequests()

	// Start pluginManager when pluginControl starts
	p.Started = true
	return nil
}

func (p *pluginControl) Stop() {
	// close(p.loadRequestsChan)
	p.Started = false
}

// Load is the public method to load a plugin into
// the LoadedPlugins array and issue an event when
// successful.
func (p *pluginControl) Load(path string) error {
	if !p.Started {
		return errors.New("Must start Controller before calling Load()")
	}

	if _, err := p.pluginManager.LoadPlugin(path); err != nil {
		return err
	}

	// defer sending event
	event := new(control_event.LoadPluginEvent)
	defer p.eventManager.Emit(event)
	return nil
}

func (p *pluginControl) Unload(pl CatalogedPlugin) error {
	err := p.pluginManager.UnloadPlugin(pl)
	if err != nil {
		return err
	}

	event := new(control_event.UnloadPluginEvent)
	defer p.eventManager.Emit(event)
	return nil
}

func (p *pluginControl) SwapPlugins(inPath string, out CatalogedPlugin) error {

	lp, err := p.pluginManager.LoadPlugin(inPath)
	if err != nil {
		return err
	}

	err = p.pluginManager.UnloadPlugin(out)
	if err != nil {
		err2 := p.pluginManager.UnloadPlugin(lp)
		if err2 != nil {
			return errors.New("failed to rollback after error" + err2.Error() + " -- " + err.Error())
		}
		return err
	}

	event := new(control_event.SwapPluginsEvent)
	defer p.eventManager.Emit(event)

	return nil
}

func (p *pluginControl) generateArgs() plugin.Arg {
	a := plugin.Arg{
		ControlPubKey: p.controlPubKey,
		PluginLogPath: "/tmp/pulse-test-plugin.log",
	}
	return a
}

// SubscribeMetricType validates the given config data, and if valid
// returns a MetricType with a config.  On error a collection of errors is returned
// either from config data processing, or the inability to find the metric.
func (p *pluginControl) SubscribeMetricType(mt core.MetricType, cd *cdata.ConfigDataNode) (core.MetricType, []error) {
	subErrs := make([]error, 0)

	m, err := p.metricCatalog.Get(mt.Namespace(), mt.Version())
	if err != nil {
		subErrs = append(subErrs, err)
		return nil, subErrs
	}

	// No metric found return error.
	if m == nil {
		subErrs = append(subErrs, errors.New(fmt.Sprintf("no metric found cannot subscribe: (%s) version(%d)", mt.Namespace(), mt.Version())))
		return nil, subErrs
	}

	if m.policy == nil {
		m.policy = cpolicy.NewPolicyNode()
	}
	ncdTable, errs := m.policy.Process(cd.Table())
	if errs != nil && errs.HasErrors() {
		return nil, errs.Errors()
	}
	m.config = cdata.FromTable(*ncdTable)

	m.Subscribe()
	e := &control_event.MetricSubscriptionEvent{
		MetricNamespace: m.Namespace(),
		Version:         m.Version(),
	}
	defer p.eventManager.Emit(e)

	return m, nil
}

// UnsubscribeMetricType unsubscribes a MetricType
// If subscriptions fall below zero we will panic.
func (p *pluginControl) UnsubscribeMetricType(mt core.MetricType) {
	err := p.metricCatalog.Unsubscribe(mt.Namespace(), mt.Version())
	if err != nil {
		// panic because if a metric falls below 0, something bad has happened
		panic(err.Error())
	}
	e := &control_event.MetricUnsubscriptionEvent{
		MetricNamespace: mt.Namespace(),
	}
	p.eventManager.Emit(e)
}

// the public interface for a plugin
// this should be the contract for
// how mgmt modules know a plugin
type CatalogedPlugin interface {
	Name() string
	Version() int
	TypeName() string
	Status() string
	LoadedTimestamp() int64
}

// the collection of cataloged plugins used
// by mgmt modules
type PluginCatalog []CatalogedPlugin

// returns a copy of the plugin catalog
func (p *pluginControl) PluginCatalog() PluginCatalog {
	table := p.pluginManager.LoadedPlugins().Table()
	pc := make([]CatalogedPlugin, len(table))
	for i, lp := range table {
		pc[i] = lp
	}
	return pc
}

func (p *pluginControl) MetricCatalog() []core.MetricType {
	var c []core.MetricType
	for p.metricCatalog.Next() {
		_, mts := p.metricCatalog.Item()
		for _, mt := range mts {
			c = append(c, mt)
		}
	}
	return c
}

func (p *pluginControl) MetricExists(mns []string, ver int) bool {
	_, err := p.metricCatalog.Get(mns, ver)
	if err == nil {
		return true
	}
	return false
}
