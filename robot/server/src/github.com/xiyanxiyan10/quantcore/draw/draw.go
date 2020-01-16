package draw

// Draw draw interface
type LineDrawer interface {
	//  PlotKLine draw line of the pic
	PlotKLine(data KlineData)
	//  PlotLine draw kline of the pic
	PlotLine(name string, data LineData)
	//  reset pic
	Reset()
	//  set path store pic
	SetPath(path string)
	// get path store pc
	GetPath() string
	// draw pic
	Draw() error
}

// GetLineDrawer ...
func GetLineDrawer() LineDrawer {
	var draw LineService
	draw.kline = []KlineData{}
	draw.line = make(map[string][]LineData)
	return &draw
}
