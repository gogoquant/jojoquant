package trader

import (
	"errors"
	"fmt"
	"github.com/robertkrimen/otto"
	goback "github.com/xiyanxiyan10/gobacktest"
	"github.com/xiyanxiyan10/samaritan/api"
	"github.com/xiyanxiyan10/samaritan/config"
	"github.com/xiyanxiyan10/samaritan/constant"
	"github.com/xiyanxiyan10/samaritan/marry"
	"github.com/xiyanxiyan10/samaritan/model"
	"gopkg.in/logger.v1"
	"time"
)

// Trader Variable
var (
	Executor      = make(map[int64]*Global)
	errHalt       = fmt.Errorf("HALT")
	exchangeMaker = map[string]func(api.Option) api.Exchange{
		constant.Huobi:        api.NewHuobi,
		constant.CoinBacktest: api.NewCoinBacktest,
	}
)

func init() {
	globaldataGram := goback.NewDataGramMaster(config.GetConfs())
	err := globaldataGram.Connect()
	if err != nil {
		log.Errorf("DataGram connect fail (%s)", err.Error())
		globaldataGram = nil
		return
	}
	err = globaldataGram.Start()
	if err != nil {
		log.Errorf("DataGram start fail (%s)", err.Error())
		globaldataGram = nil
		return
	}
	goback.SetDataGramMaster(globaldataGram)
}

// Global ...
type Global struct {
	back     *goback.Backtest
	datagram *goback.DataGramMaster

	model.Trader
	Logger model.Logger
	Ctx    *otto.Otto
	es     []api.Exchange

	tasks     []task
	execed    bool
	statusLog string
}

// GetTraderStatus ...
func GetTraderStatus(id int64) (status int64) {
	if t, ok := Executor[id]; ok && t != nil {
		status = t.Status
	}
	return
}

// Switch ...
func Switch(id int64) (err error) {
	if GetTraderStatus(id) > 0 {
		return stop(id)
	}
	return run(id)
}

// GetTrader
func GetTrader(id int64) (global *Global, err error) {
	log.Infof("%s", Executor)
	if t, ok := Executor[id]; !ok || t == nil {
		return nil, fmt.Errorf("Can not found the Trader")
	}
	return Executor[id], nil
}

// initialize ...
func initialize(id int64) (trader Global, err error) {
	//stop if in running
	if t := Executor[id]; t != nil && t.Status > 0 {
		return
	}

	err = model.DB.First(&trader.Trader, id).Error
	if err != nil {
		return
	}
	self, err := model.GetUserByID(trader.UserID)
	if err != nil {
		return
	}
	if trader.AlgorithmID <= 0 {
		err = fmt.Errorf("Please select a algorithm")
		return
	}
	err = model.DB.First(&trader.Algorithm, trader.AlgorithmID).Error
	if err != nil {
		return
	}
	es, err := self.GetTraderExchanges(trader.ID)
	if err != nil {
		return
	}
	exchangeCheckTable := make(map[string]int)
	for _, e := range es {
		exchangeCheckTable[e.Type] += 1
		if exchangeCheckTable[e.Type] > 1 {
			err = errors.New("a trader init with an exchange twice")
			return
		}
	}

	//Install exchange and portfolio into backtest
	back := goback.NewBacktest(config.GetConfs())
	back.SetId(fmt.Sprintf("%d", trader.ID))
	portfolio := goback.NewPortfolio()
	back.SetPortfolio(portfolio)
	exchange := goback.NewExchange()
	back.SetExchange(exchange)
	trader.back = back

	trader.Logger = model.Logger{
		TraderID:     trader.ID,
		ExchangeType: "global",
	}
	trader.tasks = []task{}
	trader.Ctx = otto.New()
	trader.Ctx.Interrupt = make(chan func(), 1)
	for _, c := range constant.Consts {
		trader.Ctx.Set(c, c)
	}
	//set
	incomeing := api.NewIncomingHandler(20)
	for _, e := range es {
		if maker, ok := exchangeMaker[e.Type]; ok {
			opt := api.Option{
				TraderID:  trader.ID,
				Type:      e.Type,
				Name:      e.Name,
				AccessKey: e.AccessKey,
				SecretKey: e.SecretKey,
				Mode:      trader.Mode,
				// Ctx:       trader.Ctx,
				In: incomeing,
				// share by container
				Back: trader.back,
			}

			coinbackmaker, ok := exchangeMaker[constant.CoinBacktest]
			if !ok {
				err = fmt.Errorf("get backtest module fail")
				return
			}

			switch trader.Mode {
			case constant.MODE_ONLINE:
				exchange := maker(opt)
				trader.es = append(trader.es, exchange)
			case constant.MODE_OFFLINE:
				exchange := coinbackmaker(opt)
				trader.es = append(trader.es, exchange)
			case constant.MODE_HALFLINE:
				exchange := coinbackmaker(opt)
				trader.es = append(trader.es, exchange)
			default:
				err = fmt.Errorf("unknown mode")
				return
			}
		}

	}
	if len(trader.es) == 0 {
		err = fmt.Errorf("Please add at least one exchange")
		return
	}

	//Register marry handler
	marryStore := marry.MarryStore()
	for stockType, Handler := range marryStore {
		trader.back.SetMarry(stockType, Handler)
	}

	trader.Ctx.Set("Global", &trader)
	trader.Ctx.Set("G", &trader)
	trader.Ctx.Set("Exchange", trader.es[0])
	trader.Ctx.Set("E", trader.es[0])
	trader.Ctx.Set("Exchanges", trader.es)
	trader.Ctx.Set("Es", trader.es)

	//register math tool
	trader.Ctx.Set("Math", Mathtools)

	return
}

// run ...
func run(id int64) (err error) {
	trader, err := initialize(id)
	if err != nil {
		return
	}

	//start gobacktest and exchange
	err = trader.back.Start()

	//start exchange filebeats
	for _, e := range trader.es {
		if err = e.Start(); err != nil {
			return err
		}
	}

	if err != nil {
		trader.Logger.Log(constant.ERROR, "", 0.0, 0.0, err)
	}

	go func() {
		defer func() {
			if err := recover(); err != nil && err != errHalt {
				trader.Logger.Log(constant.ERROR, "", 0.0, 0.0, err)
			}
			if exit, err := trader.Ctx.Get("exit"); err == nil && exit.IsFunction() {
				if _, err := exit.Call(exit); err != nil {
					trader.Logger.Log(constant.ERROR, "", 0.0, 0.0, err)
				}
			}
			trader.Status = 0
		}()
		trader.LastRunAt = time.Now()
		trader.Status = 1
		if _, err := trader.Ctx.Run(trader.Algorithm.Script); err != nil {
			trader.Logger.Log(constant.ERROR, "", 0.0, 0.0, err)
		}
		if main, err := trader.Ctx.Get("main"); err != nil || !main.IsFunction() {
			trader.Logger.Log(constant.ERROR, "", 0.0, 0.0, "Can not get the main function")
		} else {
			if _, err := main.Call(main); err != nil {
				trader.Logger.Log(constant.ERROR, "", 0.0, 0.0, err)
			}
		}
	}()
	Executor[trader.ID] = &trader
	return
}

// getStatus ...
func getStatus(id int64) (status string) {
	if t := Executor[id]; t != nil {
		status = t.statusLog
	}
	return
}

// stop ...
func stop(id int64) (err error) {
	//start gobacktest and exchange

	if t, ok := Executor[id]; !ok || t == nil {
		return fmt.Errorf("Can not found the Trader")
	}

	//stop exchange filebeat
	for _, e := range Executor[id].es {
		if err = e.Stop(); err != nil {
			log.Errorf("Exchange (%s) stop fail", e.GetName())
			return err
		}
	}

	Executor[id].Ctx.Interrupt <- func() { panic(errHalt) }
	Executor[id].back.Stop()

	return
}

// clean ...
func clean(userID int64) {
	for _, t := range Executor {
		if t != nil && t.UserID == userID {
			stop(t.ID)
		}
	}
}
