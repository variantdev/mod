package maputil

import "fmt"

func CastKeysToStrings(s interface{}) (map[string]interface{}, error) {
	new := map[string]interface{}{}
	switch src := s.(type) {
	case map[interface{}]interface{}:
		for k, v := range src {
			var str_k string
			switch typed_k := k.(type) {
			case string:
				str_k = typed_k
			default:
				return nil, fmt.Errorf("unexpected type of key in map: expected string, got %T: value=%v, map=%v", typed_k, typed_k, src)
			}

			casted_v, err := recursivelyStringifyMapKey(v)
			if err != nil {
				return nil, err
			}

			new[str_k] = casted_v
		}
	case map[string]interface{}:
		for k, v := range src {
			casted_v, err := recursivelyStringifyMapKey(v)
			if err != nil {
				return nil, err
			}

			new[k] = casted_v
		}
	default:
		return nil, fmt.Errorf("unexpected type: value=%v, type=%T", src, src)
	}
	return new, nil
}

func RecursivelyCastKeysToStrings(s interface{}) (interface{}, error) {
	switch src := s.(type) {
	case []interface{}:
		dst := []interface{}{}
		for _, item := range src {
			res, err := CastKeysToStrings(item)
			if err != nil {
				return nil, err
			}
			dst = append(dst, res)
		}

		return dst, nil
	default:
		return CastKeysToStrings(s)
	}
	return nil, nil
}

func recursivelyStringifyMapKey(v interface{}) (interface{}, error) {
	var casted_v interface{}
	switch typed_v := v.(type) {
	case map[interface{}]interface{}, map[string]interface{}:
		tmp, err := CastKeysToStrings(typed_v)
		if err != nil {
			return nil, err
		}
		casted_v = tmp
	case []interface{}:
		a := []interface{}{}
		for i := range typed_v {
			res, err := recursivelyStringifyMapKey(typed_v[i])
			if err != nil {
				return nil, err
			}
			a = append(a, res)
		}
		casted_v = a
	default:
		casted_v = typed_v
	}
	return casted_v, nil
}

func Set(m map[string]interface{}, key []string, value interface{}) map[string]interface{} {
	if len(key) == 0 {
		panic(fmt.Errorf("bug: unexpected length of key: %d", len(key)))
	}

	k := key[0]

	if len(key) == 1 {
		m[k] = value
		return m
	}

	remain := key[1:]

	nested, ok := m[k]
	if !ok {
		new_m := map[string]interface{}{}
		nested = Set(new_m, remain, value)
	}

	m[k] = nested

	return m
}
