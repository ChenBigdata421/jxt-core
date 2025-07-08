@startuml
class Config {
    +Load() error
    +Get(string) Value
    +Watch(string, func(Value)) error
}

class Options {
    +Encoder
    +Reader
    +Source[]
}

class Value {
    +Bool() bool
    +Int() int
    +String() string
    +Map() map[string]Value
}

interface Source {
    +Read() []byte
    +Watch() chan[]byte
}

class FileSource {
    +path string
    +Read() []byte
    +Watch() chan[]byte
}

class MemorySource {
    +data []byte
    +Read() []byte
}

interface Loader {
    +Load(Config) error
}

class MemoryLoader {
    +data map[string]interface{}
    +Load(Config) error
}

interface Encoder {
    +Encode(map) []byte
    +Decode([]byte) map
}

class JSONEncoder {
    +Encode() []byte
    +Decode() map
}

Config --* Options : 配置选项
Config --* Loader : 使用加载器
Config --> Source : 聚合数据源
Config --> Value : 生成
Source <|.. FileSource : 文件实现
Source <|.. MemorySource : 内存实现
Loader <|.. MemoryLoader : 内存加载
Encoder <|.. JSONEncoder : JSON编码
Options --* Encoder : 包含编码器
@enduml