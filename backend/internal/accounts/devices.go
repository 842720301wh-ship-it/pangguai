package accounts

import "errors"

// AddDevice updates encrypted account data atomically, preserving login tokens.
func (s *Store) AddDevice(id string, device SavedDevice) error {
	if device.ID == "" || len(device.ID) > 128 || device.Name == "" {
		return errors.New("设备信息无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.read()
	if err != nil {
		return err
	}
	for i := range records {
		if records[i].Profile.ID != id {
			continue
		}
		for j := range records[i].Devices {
			if records[i].Devices[j].ID == device.ID {
				records[i].Devices[j] = device
				return s.write(records)
			}
		}
		if len(records[i].Devices) >= 50 {
			return errors.New("最多保存 50 台扫码设备")
		}
		records[i].Devices = append(records[i].Devices, device)
		return s.write(records)
	}
	return errors.New("账号不存在")
}
