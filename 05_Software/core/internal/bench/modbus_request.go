package bench

import "nilo.local/nilo/internal/modbus"

// ModbusRequest addresses raw wire offsets; map-based typed values belong to the map service.
type ModbusRequest struct {
	Function    int   `json:"function"`
	Unit        int   `json:"unit"`
	Address     int   `json:"address"`
	Count       int   `json:"count"`
	Values      []int `json:"values"`
	AndMask     *int  `json:"and_mask,omitempty"`
	OrMask      *int  `json:"or_mask,omitempty"`
	ReadAddress *int  `json:"read_address,omitempty"`
	ReadCount   int   `json:"read_count,omitempty"`
}

var modbusLimits = map[int]int{1: 2000, 2: 2000, 3: 125, 4: 125, 5: 1, 6: 1, 15: 1968, 16: 123, 22: 1, 23: 121}

func canonicalModbusRequest(request ModbusRequest) (ModbusRequest, error) {
	limit := modbusLimits[request.Function]
	if limit == 0 || request.Unit < 0 || request.Unit > 255 || request.Address < 0 || request.Address > 65535 || request.Count < 1 || request.Count > limit || request.Address+request.Count > 65536 {
		return ModbusRequest{}, ErrInvalid
	}
	if request.Function == 22 {
		if request.AndMask == nil || request.OrMask == nil || *request.AndMask < 0 || *request.AndMask > 65535 || *request.OrMask < 0 || *request.OrMask > 65535 || request.ReadAddress != nil || request.ReadCount != 0 || len(request.Values) != 0 {
			return ModbusRequest{}, ErrInvalid
		}
		return request, nil
	}
	if request.Function == 23 {
		if request.ReadAddress == nil || *request.ReadAddress < 0 || *request.ReadAddress > 65535 || request.ReadCount < 1 || request.ReadCount > 125 || *request.ReadAddress+request.ReadCount > 65536 || request.AndMask != nil || request.OrMask != nil {
			return ModbusRequest{}, ErrInvalid
		}
	} else if request.AndMask != nil || request.OrMask != nil || request.ReadAddress != nil || request.ReadCount != 0 {
		return ModbusRequest{}, ErrInvalid
	}
	write := request.Function == 5 || request.Function == 6 || request.Function == 15 || request.Function == 16 || request.Function == 23
	if write != (len(request.Values) == request.Count) {
		return ModbusRequest{}, ErrInvalid
	}
	for _, value := range request.Values {
		if request.Function == 5 || request.Function == 15 {
			if value != 0 && value != 1 {
				return ModbusRequest{}, ErrInvalid
			}
		} else if value < 0 || value > 65535 {
			return ModbusRequest{}, ErrInvalid
		}
	}
	request.Values = append([]int{}, request.Values...)
	return request, nil
}

func validateModbusTarget(request ModbusRequest, endpoint modbus.Endpoint) error {
	if endpoint.Transport == "tcp" {
		if endpoint.Network != nil && request.Unit >= endpoint.Network.UnitMin && request.Unit <= endpoint.Network.UnitMax {
			return nil
		}
		return ErrInvalid
	}
	if request.Unit < 1 || request.Unit > 247 {
		return ErrInvalid
	}
	if endpoint.Transport == "rtu_tcp" && (endpoint.Network == nil || request.Unit < endpoint.Network.UnitMin || request.Unit > endpoint.Network.UnitMax) {
		return ErrInvalid
	}
	return nil
}
