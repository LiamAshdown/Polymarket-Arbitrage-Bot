package signals

type Signal interface {
	Update(tokenID string, price float64)

	Check(tokenID string) bool

	IsReady(tokenID string) bool

	Name() string
}

type SignalConfig struct {
	RSIEnabled    bool
	RSIPeriod     int
	RSIOverbought float64
	RSIOversold   float64

	VolumeEnabled   bool
	VolumePeriod    int
	VolumeThreshold float64
}

type Manager struct {
	signals      []Signal
	volumeSignal *VolumeWeight
}

func NewManager(config SignalConfig) *Manager {
	manager := &Manager{
		signals: make([]Signal, 0),
	}

	if config.RSIEnabled {
		manager.signals = append(manager.signals, NewRSI(config.RSIPeriod, config.RSIOverbought, config.RSIOversold))
	}

	if config.VolumeEnabled {
		manager.volumeSignal = NewVolumeWeight(config.VolumePeriod, config.VolumeThreshold)
		manager.signals = append(manager.signals, manager.volumeSignal)
	}

	return manager
}

func (m *Manager) Update(tokenID string, price float64) {
	for _, signal := range m.signals {
		signal.Update(tokenID, price)
	}
}

func (m *Manager) UpdateVolume(tokenID string, volume float64) {
	if m.volumeSignal != nil {
		m.volumeSignal.UpdateVolume(tokenID, volume)
	}
}

func (m *Manager) CheckAll(tokenIDs ...string) bool {
	for _, signal := range m.signals {
		for _, tokenID := range tokenIDs {
			if !signal.IsReady(tokenID) || !signal.Check(tokenID) {
				return false
			}
		}
	}
	return true
}

func (m *Manager) AnyReady(tokenID string) bool {
	for _, signal := range m.signals {
		if signal.IsReady(tokenID) {
			return true
		}
	}
	return false
}
