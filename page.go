package main

import (
	"fmt"

	"github.com/divan/graphx/layout"
	"github.com/gopherjs/vecty"
	"github.com/gopherjs/vecty/elem"
	"github.com/gopherjs/vecty/event"
	"github.com/gopherjs/vecty/prop"
	"github.com/status-im/whispervis/network"
	"github.com/status-im/whispervis/widgets"
)

// Page is our main page component.
type Page struct {
	vecty.Core

	layout *layout.Layout

	webgl *WebGLScene

	loaded       bool
	isSimulating bool

	loader           *widgets.Loader
	network          *widgets.NetworkSelector
	forceEditor      *widgets.ForceEditor
	graphics         *widgets.Graphics
	simulationWidget *widgets.Simulation
	statsWidget      *widgets.Stats

	statsPage *StatsPage
	faqPage   *FAQPage

	simulation *Simulation
	activeView string
}

// NewPage creates and inits new app page.
func NewPage() *Page {
	page := &Page{}

	// preload defaults
	page.activeView = View3D

	// init widgets
	page.loader = widgets.NewLoader()
	page.webgl = NewWebGLScene(page.onWebGLReady)
	page.network = widgets.NewNetworkSelector(page.onNetworkChange)
	page.forceEditor = widgets.NewForceEditor(page.onForcesApply)
	page.graphics = widgets.NewGraphics(page.webgl)
	page.simulationWidget = widgets.NewSimulation("http://localhost:8084", page.startSimulation, page.replaySimulation, page.stepSimulation)
	page.statsWidget = widgets.NewStats()
	page.statsPage = NewStatsPage()
	page.faqPage = NewFAQPage()
	return page
}

// Render implements the vecty.Component interface.
func (p *Page) Render() vecty.ComponentOrHTML {
	return elem.Body(
		elem.Div(
			vecty.Markup(
				vecty.Class("columns"),
			),
			// Left sidebar
			elem.Div(
				vecty.Markup(
					vecty.Class("column", "is-narrow"),
					vecty.Style("width", "300px"),
				),
				p.header(),
				elem.Div(
					vecty.Markup(
						vecty.MarkupIf(p.isSimulating,
							vecty.Attribute("disabled", "true"),
						),
					),
					p.network,
					p.forceEditor,
					p.graphics,
				),
				elem.Div(
					vecty.Markup(
						vecty.MarkupIf(!p.loaded, vecty.Style("visibility", "hidden")),
					),
					p.simulationWidget,
					elem.Div(
						vecty.Markup(
							vecty.MarkupIf(p.isSimulating,
								vecty.Attribute("disabled", "true"),
							),
						),
						p.statsWidget,
					),
				),
			),
			// Right page section
			elem.Div(
				vecty.Markup(
					vecty.Class("column"),
				),
				p.renderTabs(),
				elem.Div(
					/*
						we use display:none property to hide WebGL instead of mounting/unmounting,
						because we want to create only one WebGL context and reuse it. Plus,
						WebGL takes time to initialize, so it can do it being hidden.
					*/
					vecty.Markup(
						vecty.MarkupIf(!p.loaded || p.activeView != View3D,
							vecty.Class("is-invisible"),
							vecty.Style("height", "0px"),
						),
					),
					p.webgl,
				),
				vecty.If(p.activeView == ViewStats,
					p.statsPage,
				),
				vecty.If(p.activeView == ViewFAQ,
					p.faqPage,
				),
				vecty.If(!p.loaded,
					elem.Div(
						vecty.Markup(
							vecty.Class("has-text-centered"),
						),
						p.loader,
					),
				),
			),
		),
		vecty.Markup(
			event.KeyDown(p.KeyListener),
			event.MouseMove(p.MouseMoveListener),
			event.VisibilityChange(p.VisibilityListener),
		),
	)
}

// onForcesApply executes when Force Editor click is fired.
func (p *Page) onForcesApply() {
	if !p.loaded {
		return
	}
	p.UpdateGraph()
}

func (p *Page) onNetworkChange(network *network.Network) {
	fmt.Println("Network changed:", network)

	// reset view on network switch
	p.switchView(View3D)

	// reset simulation and stats
	p.simulation = nil
	p.simulationWidget.Reset()
	p.statsWidget.Reset()

	config := p.forceEditor.Config()
	p.layout = layout.New(network.Data, config.Config)

	// set forced positions if found in network
	if network.Positions != nil {
		p.layout.SetPositions(network.Positions)
		go p.RecreateObjects()
		return
	}

	// else, recalculate positions
	go p.UpdateGraph()
}

// startSimulation is called on the end of each simulation round.
// Returns numnber of timesteps for the simulation.
// TODO(divan): maybe sim widget need to have access to whole simulation?
func (p *Page) startSimulation() (int, error) {
	p.isSimulating = true
	vecty.Rerender(p)

	defer func() {
		p.isSimulating = false
		vecty.Rerender(p)
	}()

	backend := p.simulationWidget.Address()
	ttl := p.simulationWidget.TTL()
	sim, err := p.runSimulation(backend, ttl)
	if err != nil {
		return 0, err
	}

	// calculate stats and update stats widget
	stats := p.RecalculateStats(sim.plog)
	p.statsWidget.Update(stats)

	sim.stats = stats
	p.simulation = sim
	p.statsPage.UpdateStats(p.network.Current().Data, p.simulation.plog)

	p.replaySimulation()
	return len(sim.plog.Timestamps), nil
}

// replaySimulation animates last simulation.
func (p *Page) replaySimulation() {
	if p.simulation == nil {
		return
	}
	p.webgl.AnimatePropagation(p.simulation.plog)
}

func (p *Page) header() *vecty.HTML {
	return elem.Section(
		elem.Image(
			vecty.Markup(
				vecty.Style("padding-top", "5px"),
				prop.Src("images/status.png"),
			),
		),
		elem.Heading2(
			vecty.Markup(
				vecty.Class("title", "has-text-weight-light"),
			),
			vecty.Text("Whisper Simulator"),
		),
		elem.Heading6(
			vecty.Markup(
				vecty.Class("subtitle", "has-text-weight-light"),
			),
			vecty.Text("This simulator shows message propagation in the Whisper network."),
		),
	)
}

// onWebGLReady is executed when WebGL context is up and ready to render scene.
func (p *Page) onWebGLReady() {
	p.onNetworkChange(p.network.Current())
}

func (p *Page) renderTabs() *vecty.HTML {
	return elem.Div(
		vecty.Markup(
			vecty.Class("tabs", "is-marginless", "is-boxed", "is-fullwidth"),
		),
		elem.UnorderedList(
			elem.ListItem(
				vecty.Markup(
					vecty.MarkupIf(p.activeView == View3D,
						vecty.Class("is-active"),
					),
					event.Click(p.onTabSwitch(View3D)),
				),
				elem.Anchor(
					vecty.Text("3D view"),
				),
			),
			elem.ListItem(
				vecty.Markup(
					vecty.MarkupIf(p.activeView == ViewStats,
						vecty.Class("is-active"),
					),
					event.Click(p.onTabSwitch(ViewStats)),
				),
				elem.Anchor(
					vecty.Text("Stats view"),
				),
			),
			elem.ListItem(
				vecty.Markup(
					vecty.MarkupIf(p.activeView == ViewFAQ,
						vecty.Class("is-active"),
					),
					event.Click(p.onTabSwitch(ViewFAQ)),
				),
				elem.Anchor(
					vecty.Text("FAQ"),
				),
			),
		),
	)
}

// stepSimulation animates a single step from the last simulation.
func (p *Page) stepSimulation(step int) {
	if p.simulation == nil {
		return
	}
	p.webgl.AnimateOneStep(p.simulation.plog, step)
}
