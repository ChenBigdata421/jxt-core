package json

import (
	"testing"
)

type TestStruct struct {
	Name  string `json:"name"`
	Age   int    `json:"age"`
	Email string `json:"email,omitempty"`
}

func TestMarshal(t *testing.T) {
	obj := TestStruct{
		Name: "Alice",
		Age:  30,
	}

	data, err := Marshal(obj)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Marshal returned empty data")
	}

	expected := `{"name":"Alice","age":30}`
	if string(data) != expected {
		t.Errorf("Marshal result mismatch: got %s, want %s", string(data), expected)
	}
}

func TestUnmarshal(t *testing.T) {
	data := []byte(`{"name":"Bob","age":25,"email":"bob@example.com"}`)

	var obj TestStruct
	err := Unmarshal(data, &obj)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if obj.Name != "Bob" {
		t.Errorf("Name mismatch: got %s, want Bob", obj.Name)
	}
	if obj.Age != 25 {
		t.Errorf("Age mismatch: got %d, want 25", obj.Age)
	}
	if obj.Email != "bob@example.com" {
		t.Errorf("Email mismatch: got %s, want bob@example.com", obj.Email)
	}
}

func TestMarshalToString(t *testing.T) {
	obj := TestStruct{
		Name: "Charlie",
		Age:  35,
	}

	str, err := MarshalToString(obj)
	if err != nil {
		t.Fatalf("MarshalToString failed: %v", err)
	}

	if str == "" {
		t.Fatal("MarshalToString returned empty string")
	}

	expected := `{"name":"Charlie","age":35}`
	if str != expected {
		t.Errorf("MarshalToString result mismatch: got %s, want %s", str, expected)
	}
}

func TestUnmarshalFromString(t *testing.T) {
	str := `{"name":"David","age":40,"email":"david@example.com"}`

	var obj TestStruct
	err := UnmarshalFromString(str, &obj)
	if err != nil {
		t.Fatalf("UnmarshalFromString failed: %v", err)
	}

	if obj.Name != "David" {
		t.Errorf("Name mismatch: got %s, want David", obj.Name)
	}
	if obj.Age != 40 {
		t.Errorf("Age mismatch: got %d, want 40", obj.Age)
	}
	if obj.Email != "david@example.com" {
		t.Errorf("Email mismatch: got %s, want david@example.com", obj.Email)
	}
}

func TestRawMessage(t *testing.T) {
	type Message struct {
		Type string     `json:"type"`
		Data RawMessage `json:"data"`
	}

	// 序列化
	rawData := []byte(`{"name":"Eve","age":28}`)
	msg := Message{
		Type: "user",
		Data: rawData,
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// 反序列化
	var msg2 Message
	err = Unmarshal(data, &msg2)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if msg2.Type != "user" {
		t.Errorf("Type mismatch: got %s, want user", msg2.Type)
	}

	// 验证 RawMessage 保留了原始 JSON
	var userData TestStruct
	err = Unmarshal(msg2.Data, &userData)
	if err != nil {
		t.Fatalf("Unmarshal RawMessage failed: %v", err)
	}

	if userData.Name != "Eve" {
		t.Errorf("Name mismatch: got %s, want Eve", userData.Name)
	}
	if userData.Age != 28 {
		t.Errorf("Age mismatch: got %d, want 28", userData.Age)
	}
}

func TestRoundTrip(t *testing.T) {
	original := TestStruct{
		Name:  "Frank",
		Age:   45,
		Email: "frank@example.com",
	}

	// 序列化
	data, err := Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// 反序列化
	var result TestStruct
	err = Unmarshal(data, &result)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// 验证
	if result.Name != original.Name {
		t.Errorf("Name mismatch: got %s, want %s", result.Name, original.Name)
	}
	if result.Age != original.Age {
		t.Errorf("Age mismatch: got %d, want %d", result.Age, original.Age)
	}
	if result.Email != original.Email {
		t.Errorf("Email mismatch: got %s, want %s", result.Email, original.Email)
	}
}

func TestMapSerialization(t *testing.T) {
	// 测试 map[string]interface{} 序列化（Go 1.24 Swiss Tables 兼容性）
	data := map[string]interface{}{
		"name":  "Grace",
		"age":   50,
		"email": "grace@example.com",
	}

	// 序列化
	jsonData, err := Marshal(data)
	if err != nil {
		t.Fatalf("Marshal map failed: %v", err)
	}

	// 反序列化
	var result map[string]interface{}
	err = Unmarshal(jsonData, &result)
	if err != nil {
		t.Fatalf("Unmarshal map failed: %v", err)
	}

	// 验证
	if result["name"] != "Grace" {
		t.Errorf("Name mismatch: got %v, want Grace", result["name"])
	}
	if result["age"] != float64(50) { // JSON 数字默认解析为 float64
		t.Errorf("Age mismatch: got %v, want 50", result["age"])
	}
	if result["email"] != "grace@example.com" {
		t.Errorf("Email mismatch: got %v, want grace@example.com", result["email"])
	}
}

// 性能测试
func BenchmarkMarshal(b *testing.B) {
	obj := TestStruct{
		Name:  "Benchmark",
		Age:   100,
		Email: "bench@example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Marshal(obj)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	data := []byte(`{"name":"Benchmark","age":100,"email":"bench@example.com"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var obj TestStruct
		err := Unmarshal(data, &obj)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalToString(b *testing.B) {
	obj := TestStruct{
		Name:  "Benchmark",
		Age:   100,
		Email: "bench@example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := MarshalToString(obj)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalFromString(b *testing.B) {
	str := `{"name":"Benchmark","age":100,"email":"bench@example.com"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var obj TestStruct
		err := UnmarshalFromString(str, &obj)
		if err != nil {
			b.Fatal(err)
		}
	}
}

