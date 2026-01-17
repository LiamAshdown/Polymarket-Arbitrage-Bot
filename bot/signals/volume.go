package signals

import "sync"

// VolumeWeight ensures minimum market activity before allowing orders
// Blocks trading when volume < threshold * moving average
type VolumeWeight struct {
	period    int
	threshold float64

	data  map[string]*volumeData
	mutex sync.RWMutex
}

type volumeData struct {
	volumes     []float64
	avgVolume   float64
	initialized bool
}

func NewVolumeWeight(period int, threshold float64) *VolumeWeight {
	return &VolumeWeight{
		period:    period,
		threshold: threshold,
		data:      make(map[string]*volumeData),
	}
}

func (v *VolumeWeight) Name() string {
	return "VolumeWeight"
}

func (v *VolumeWeight) UpdateVolume(tokenID string, volume float64) {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if _, exists := v.data[tokenID]; !exists {
		v.data[tokenID] = &volumeData{
			volumes: make([]float64, 0, v.period),
		}
	}

	data := v.data[tokenID]
	data.volumes = append(data.volumes, volume)

	if len(data.volumes) < v.period {
		return
	}

	if len(data.volumes) > v.period {
		data.volumes = data.volumes[1:]
	}

	// Calculate simple moving average
	var sum float64
	for _, vol := range data.volumes {
		sum += vol
	}
	data.avgVolume = sum / float64(len(data.volumes))
	data.initialized = true
}

func (v *VolumeWeight) Update(tokenID string, price float64) {
}

func (v *VolumeWeight) Check(tokenID string) bool {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	data, exists := v.data[tokenID]
	if !exists || !data.initialized || len(data.volumes) == 0 {
		return false
	}

	currentVolume := data.volumes[len(data.volumes)-1]
	requiredVolume := v.threshold * data.avgVolume

	return currentVolume >= requiredVolume
}

func (v *VolumeWeight) IsReady(tokenID string) bool {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	data, exists := v.data[tokenID]
	return exists && data.initialized
}
