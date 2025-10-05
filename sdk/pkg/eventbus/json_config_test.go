package eventbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarshalToString 测试序列化为字符串
func TestMarshalToString(t *testing.T) {
	// 使用简单的结构体避免 jsoniter 的 map 序列化问题
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	str, err := MarshalToString(data)
	require.NoError(t, err)
	assert.Contains(t, str, "test")
	assert.Contains(t, str, "30")
}

// TestUnmarshalFromString 测试从字符串反序列化
func TestUnmarshalFromString(t *testing.T) {
	str := `{"name":"test","age":30}`

	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	var result TestData
	err := UnmarshalFromString(str, &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 30, result.Age)
}

// TestMarshal 测试序列化
func TestMarshal(t *testing.T) {
	data := map[string]interface{}{
		"name": "test",
		"age":  30,
	}

	bytes, err := Marshal(data)
	require.NoError(t, err)
	assert.NotEmpty(t, bytes)
	assert.Contains(t, string(bytes), "test")
}

// TestUnmarshal 测试反序列化
func TestUnmarshal(t *testing.T) {
	bytes := []byte(`{"name":"test","age":30}`)

	var result map[string]interface{}
	err := Unmarshal(bytes, &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["name"])
	assert.Equal(t, float64(30), result["age"])
}

// TestMarshalFast 测试快速序列化
func TestMarshalFast(t *testing.T) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	bytes, err := MarshalFast(data)
	require.NoError(t, err)
	assert.NotEmpty(t, bytes)
}

// TestUnmarshalFast 测试快速反序列化
func TestUnmarshalFast(t *testing.T) {
	bytes := []byte(`{"name":"test","age":30}`)

	var result map[string]interface{}
	err := UnmarshalFast(bytes, &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["name"])
}

// TestJSON_RoundTrip 测试完整的序列化反序列化流程
func TestJSON_RoundTrip(t *testing.T) {
	original := map[string]interface{}{
		"string": "value",
		"number": 42,
		"bool":   true,
		"array":  []int{1, 2, 3},
		"nested": map[string]string{
			"key": "value",
		},
	}

	// 序列化
	bytes, err := Marshal(original)
	require.NoError(t, err)

	// 反序列化
	var result map[string]interface{}
	err = Unmarshal(bytes, &result)
	require.NoError(t, err)

	// 验证
	assert.Equal(t, "value", result["string"])
	assert.Equal(t, float64(42), result["number"])
	assert.Equal(t, true, result["bool"])
}

// TestJSONFast_RoundTrip 测试快速序列化反序列化流程
func TestJSONFast_RoundTrip(t *testing.T) {
	type TestData struct {
		String string `json:"string"`
		Number int    `json:"number"`
	}
	original := TestData{String: "value", Number: 42}

	// 快速序列化
	bytes, err := MarshalFast(original)
	require.NoError(t, err)

	// 快速反序列化
	var result TestData
	err = UnmarshalFast(bytes, &result)
	require.NoError(t, err)

	// 验证
	assert.Equal(t, "value", result.String)
	assert.Equal(t, 42, result.Number)
}

// TestMarshalToString_Error 测试序列化错误
func TestMarshalToString_Error(t *testing.T) {
	// 创建一个无法序列化的对象（channel 类型）
	invalidData := make(chan int)

	_, err := MarshalToString(invalidData)
	assert.Error(t, err)
}

// TestUnmarshalFromString_Error 测试反序列化错误
func TestUnmarshalFromString_Error(t *testing.T) {
	invalidJSON := `{"name": invalid}`

	var result map[string]interface{}
	err := UnmarshalFromString(invalidJSON, &result)
	assert.Error(t, err)
}

// TestMarshal_Struct 测试序列化结构体
func TestMarshal_Struct(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	person := Person{Name: "Alice", Age: 30}

	bytes, err := Marshal(person)
	require.NoError(t, err)
	assert.Contains(t, string(bytes), "Alice")
	assert.Contains(t, string(bytes), "30")
}

// TestUnmarshal_Struct 测试反序列化到结构体
func TestUnmarshal_Struct(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	bytes := []byte(`{"name":"Bob","age":25}`)

	var person Person
	err := Unmarshal(bytes, &person)
	require.NoError(t, err)
	assert.Equal(t, "Bob", person.Name)
	assert.Equal(t, 25, person.Age)
}

// TestJSON_Variables 测试JSON变量
func TestJSON_Variables(t *testing.T) {
	assert.NotNil(t, JSON)
	assert.NotNil(t, JSONFast)
	assert.NotNil(t, JSONDefault)
}

// TestRawMessage 测试RawMessage类型
func TestRawMessage(t *testing.T) {
	type Message struct {
		Type string     `json:"type"`
		Data RawMessage `json:"data"`
	}

	// 创建消息
	msg := Message{
		Type: "test",
		Data: RawMessage(`{"key":"value"}`),
	}

	// 序列化
	bytes, err := Marshal(msg)
	require.NoError(t, err)

	// 反序列化
	var result Message
	err = Unmarshal(bytes, &result)
	require.NoError(t, err)

	assert.Equal(t, "test", result.Type)
	assert.Contains(t, string(result.Data), "key")
}

// TestMarshalToString_EmptyObject 测试序列化空对象
func TestMarshalToString_EmptyObject(t *testing.T) {
	type EmptyData struct{}
	data := EmptyData{}

	str, err := MarshalToString(data)
	require.NoError(t, err)
	assert.Equal(t, "{}", str)
}

// TestUnmarshalFromString_EmptyObject 测试反序列化空对象
func TestUnmarshalFromString_EmptyObject(t *testing.T) {
	str := `{}`

	type EmptyData struct{}
	var result EmptyData
	err := UnmarshalFromString(str, &result)
	require.NoError(t, err)
}

// TestMarshal_Array 测试序列化数组
func TestMarshal_Array(t *testing.T) {
	data := []int{1, 2, 3, 4, 5}

	bytes, err := Marshal(data)
	require.NoError(t, err)
	assert.Contains(t, string(bytes), "1")
	assert.Contains(t, string(bytes), "5")
}

// TestUnmarshal_Array 测试反序列化数组
func TestUnmarshal_Array(t *testing.T) {
	bytes := []byte(`[1,2,3,4,5]`)

	var result []int
	err := Unmarshal(bytes, &result)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, result)
}

// BenchmarkMarshal 基准测试：标准序列化
func BenchmarkMarshal(b *testing.B) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Marshal(data)
	}
}

// BenchmarkMarshalFast 基准测试：快速序列化
func BenchmarkMarshalFast(b *testing.B) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = MarshalFast(data)
	}
}

// BenchmarkMarshalToString 基准测试：序列化为字符串
func BenchmarkMarshalToString(b *testing.B) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = MarshalToString(data)
	}
}
