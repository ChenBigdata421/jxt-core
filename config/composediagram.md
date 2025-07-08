@startuml
package "Config Package" {
    [Config] --> [Options]
    [Config] --> [Loader]
    [Config] --> [Source]
    [Config] --> [Value]
    
    [Options] --> [Encoder]
    [Options] --> [Reader]
    [Options] --> [Source]
    
    [Loader] --> [MemoryLoader]
    [Source] --> [FileSource]
    [Source] --> [MemorySource]
    [Encoder] --> [JSONEncoder]
}
@enduml