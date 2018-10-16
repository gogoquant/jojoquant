package gobacktest

import (
	"errors"
	"gopkg.in/logger.v1"
	//"github.com/xiyanxiyan10/samaritan/api"
)

// DP sets the the precision of rounded floating numbers
// used after calculations to format
const DP = 4 // DP

const (
	GobackRun     = 2
	GobackPending = 1
	GobackStop    = 0
)

// Reseter provides a resting interface.
type Reseter interface {
	Reset() error
}

// Backtest is the main struct which holds all elements.
type Backtest struct {
	config map[string]string

	eventCh chan EventHandler
	status  int
	name    string

	symbols []string

	data      DataHandler
	datagram  *DataGramMaster
	strategy  StrategyHandler
	portfolio PortfolioHandler
	exchange  ExecutionHandler
	statistic StatisticHandler
	marries   map[string]MarryHandler

	eventQueue []EventHandler
}

// NewBacktest
func NewBacktest(m map[string]string) *Backtest {
	return &Backtest{
		eventCh: make(chan EventHandler, 20),
		marries: make(map[string]MarryHandler),
		status:  GobackStop,
		config:  m,
	}
}

// Name ...
func (e *Backtest) Name() string {
	return e.name
}

// Name ...
func (e *Backtest) SetName(name string) {
	e.name = name
}

// SetDataGram
func (t *Backtest) SetDataGram(d *DataGramMaster) {
	t.datagram = d
}

// SetDataGram
func (t *Backtest) DataGram() *DataGramMaster {
	return t.datagram
}

// SetMarry
func (t *Backtest) SetMarry(name string, handler MarryHandler) {
	t.marries[name] = handler
}

// Marries
func (t *Backtest) Marries() map[string]MarryHandler {
	return t.marries
}

// Marry
func (t *Backtest) Marry(stockType string) (MarryHandler, bool) {
	handler, ok := t.marries[stockType]
	return handler, ok
}

// CommitOrder ...
func (t *Backtest) CommitOrder(id int) (*Fill, error) {
	fill, err := t.portfolio.CommitOrder(id)
	if err == nil && fill != nil {
		t.AddEvent(fill)
	}
	return fill, err
}

// OrdersBySymbol ...
func (t *Backtest) OrdersBySymbol(stockType string) ([]OrderEvent, bool) {
	return t.portfolio.OrdersBySymbol(stockType)
}

// CancelOneOrder ...
func (t *Backtest) CancelOrder(id int) error {
	return t.portfolio.CancelOrder(id)
}

// Subscribe..
func (t *Backtest) Subscribes() map[string]int {
	return t.portfolio.Subscribes()
}

// Start
func (t *Backtest) Start() error {
	if t.status == GobackRun {
		return errors.New("already running")
	}
	t.status = GobackRun

	/*
		err := t.influx.Connect()
		if nil != err{
			return err
		}
	*/
	// start datagram
	if t.datagram != nil {
		if err := t.datagram.Start(); err != nil {
			log.Infof("datagram run fail")
		}
	}

	// start goback
	go t.Run2Event()

	return nil
}

// Stop
func (t *Backtest) Stop() error {
	if t.status == GobackStop || t.status == GobackPending {
		return errors.New("already stop or pending")

	}
	/*
		err := t.influx.Close()
		if nil != err{
			return err
		}
	*/

	//stop datagram
	if t.datagram != nil {
		if err := t.datagram.Stop(); err != nil {
			log.Infof("datagram stop fail")
		}
	}

	//stop gobacktest
	t.status = GobackPending
	var cmd Cmd
	cmd.SetCmd("stop")
	t.AddEvent(&cmd)

	return nil
}

// New creates a default backtest with sensible defaults ready for use.
func New() *Backtest {
	return &Backtest{
		portfolio: &Portfolio{
			initialCash: 100000,
			sizeManager: &Size{DefaultSize: 100, DefaultValue: 1000},
			riskManager: &Risk{},
		},
		exchange: &Exchange{
			Symbol: "TEST",
			//Commission:  &FixedCommission{Commission: 0},
			ExchangeFee: &FixedExchangeFee{ExchangeFee: 0},
		},
		statistic: &Statistic{},
	}
}

// SetSymbols sets the symbols to include into the backtest.
func (t *Backtest) SetSymbols(symbols []string) {
	t.symbols = symbols
}

// SetData sets the data provider to be used within the backtest.
func (t *Backtest) SetData(data DataHandler) {
	t.data = data
}

// SetStrategy sets the strategy provider to be used within the backtest.
func (t *Backtest) SetStrategy(strategy StrategyHandler) {
	t.strategy = strategy
}

// SetPortfolio sets the portfolio provider to be used within the backtest.
func (t *Backtest) SetPortfolio(portfolio PortfolioHandler) {
	t.portfolio = portfolio
}

// SetExchange sets the execution provider to be used within the backtest.
func (t *Backtest) SetExchange(exchange ExecutionHandler) {
	t.exchange = exchange
}

// SetStatistic sets the statistic provider to be used within the backtest.
func (t *Backtest) SetStatistic(statistic StatisticHandler) {
	t.statistic = statistic
}

// Portfolio sets the Portfolio provider to be used within the backtest.
func (t *Backtest) Portfolio() PortfolioHandler {
	return t.portfolio
}

// Reset the backtest into a clean state with loaded data.
func (t *Backtest) Reset() error {
	t.eventQueue = nil
	t.data.Reset()
	t.portfolio.Reset()
	t.statistic.Reset()
	return nil
}

// SignalAdd Add signal event into event queue
func (t *Backtest) AddSignal(signals ...SignalEvent) error {
	for _, signal := range signals {
		t.AddEvent(signal)
	}
	return nil
}

// Stats returns the statistic handler of the backtest.
func (t *Backtest) Stats() StatisticHandler {
	return t.statistic
}

// Run starts the backtest.
func (t *Backtest) Run() error {
	// setup before the backtest runs
	err := t.setup()
	if err != nil {
		return err
	}

	// poll event queue
	for event, ok := t.nextEvent(); true; event, ok = t.nextEvent() {
		// no event in the queue
		if !ok {
			// poll data stream
			data, ok := t.data.Next()
			// no more data, exit event loop
			if !ok {
				break
			}
			// found data event, add to event stream
			t.eventQueue = append(t.eventQueue, data)
			// start new event cycle
			continue
		}

		// processing event
		err := t.eventLoop(event)
		if err != nil {
			return err
		}
		// event in queue found, add to event history
		t.statistic.TrackEvent(event)
	}

	// teardown at the end of the backtest
	err = t.teardown()
	if err != nil {
		return err
	}
	return nil
}

// Run starts the backtest to get data tick
func (t *Backtest) Run2Event() error {
	// poll event queue
	for {
		event := <-t.eventCh
		//log.Infof("Get event:")
		err, end := t.eventLoop2Event(event)
		if err != nil {
			return err
		}
		if end {
			return nil
		}
		// event in queue found, add to event history
		//t.statistic.TrackEvent(event)
	}
	return nil
}

// setup runs at the beginning of the backtest to perfom preparing operations.
func (t *Backtest) setup() error {
	// before first run, set portfolio cash
	t.portfolio.SetCash(t.portfolio.InitialCash())

	// make the data known to the strategy
	err := t.strategy.SetData(t.data)
	if err != nil {
		return err
	}

	// make the portfolio known to the strategy
	err = t.strategy.SetPortfolio(t.portfolio)
	if err != nil {
		return err
	}

	return nil
}

// teardown performs any cleaning operations at the end of the backtest.
func (t *Backtest) teardown() error {
	// no implementation yet
	return nil
}

// nextEvent gets the next event from the events queue.
func (t *Backtest) nextEvent() (e EventHandler, ok bool) {

	// if event queue empty return false
	if len(t.eventQueue) == 0 {
		return e, false
	}

	// return first element from the event queue
	e = t.eventQueue[0]
	t.eventQueue = t.eventQueue[1:]

	return e, true
}

// AddEvent
func (t *Backtest) AddEvent(e EventHandler) error {
	t.eventCh <- e
	return nil
}

// eventLoop directs the different events to their handler.
func (t *Backtest) eventLoop(e EventHandler) error {
	// type check for event type
	/*
		switch event := e.(type) {

		case DataEvent:
			// update portfolio to the last known price data
			t.portfolio.Update(event)
			// update statistics
			t.statistic.Update(event, t.portfolio)
			// check if any orders are filled before proceding
			t.exchange.OnData(event)

			// run strategy with this data event
			signals, err := t.strategy.OnData(event)
			if err != nil {
				break
			}
			for _, signal := range signals {
				t.eventQueue = append(t.eventQueue, signal)
			}

		case *Signal:
			order, err := t.portfolio.OnSignal(event, t.data)
			if err != nil {
				break
			}
			t.eventQueue = append(t.eventQueue, order)

		case *Order:
			fill, err := t.exchange.OnOrder(event, t.data)
			if err != nil {
				break
			}
			t.eventQueue = append(t.eventQueue, fill)

		case *Fill:
			transaction, err := t.portfolio.OnFill(event, t.data)
			if err != nil {
				break
			}
			t.statistic.TrackTransaction(transaction)
		}
	*/
	return nil

}

// eventLoop2Event directs the different events to their handler.
func (t *Backtest) eventLoop2Event(e EventHandler) (err error, end bool) {
	end = false

	// type check for event type
	switch event := e.(type) {

	case DataGramEvent:
		log.Infof("Get dataGram event symbol (%s) timestamp (%s)", event.Symbol(), event.Time())

		if t.datagram == nil {
			log.Infof("dataGram master not found")
		}

		//@TODO get backtest name, need CONTAINER id
		event.SetId(t.name)
		err = t.datagram.AddDataGram(event)
		if err != nil {
			end = true
		}
		end = false
		break

	case CmdEvent:
		log.Infof("Get cmd event symbol (%s) timestamp (%s)", event.Symbol(), event.Time())
		t.status = GobackStop
		err = nil
		end = true
		break

	case DataEvent:
		log.Infof("Get data event symbol (%s) timestamp (%s)", event.Symbol(), event.Time())
		// update portfolio to the last known price data
		//t.portfolio.Update(event)
		// update statistics
		//t.statistic.Update(event, t.portfolio)
		// check if any orders are filled before proceding
		//t.exchange.OnData(event)
		// marry all orders by stockType
		marry, ok := t.Marry(event.Symbol())
		if ok {
			_, err := marry.Marry(t, event)
			if err != nil {
				return err, true
			}
		}

	case *Signal:
		log.Infof("Get signal event symbol (%s) timestamp (%s)", event.Symbol(), event.Time())
		order, err := t.portfolio.OnSignal(event, t.data)
		if err != nil {
			break
		}
		t.AddEvent(order)

	case *Order:
		log.Infof("Get order event symbol (%s) timestamp (%s)", event.Symbol(), event.Time())
		//@Todo move to exchange to manger the order
		//fill, err := t.exchange.OnOrder(event, t.data)
		t.Portfolio().AddOrder(event)
		if err != nil {
			break
		}
		//t.AddEvent(fill)

	case *Fill:
		log.Infof("Get fill event symbol (%s) timestamp (%s)", event.Symbol(), event.Time())
		t.exchange.OnFill(event)
		_, err := t.portfolio.OnFill(event, t.data)
		if err != nil {
			break
		}

		//t.AddEvent(transaction)
	}
	return
}
