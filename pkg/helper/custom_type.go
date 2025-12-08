package helper

type Numeric interface {
	int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64 |
		float32 | float64
}

type Validateable interface {
	Numeric | []any | map[string]string | map[string]any |
		map[any]any | string | any |
		[]string
}
