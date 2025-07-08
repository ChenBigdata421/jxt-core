@startuml
start
:创建 Config 实例;
:初始化 Options;
:加载配置源;
:获取配置值;
repeat
    :监视配置变化;
    if (配置变化?) then (yes)
        :更新配置值;
    endif
    :返回配置值;
repeat while (用户请求?)
end
if (关闭请求?) then (yes)
    :关闭 Config;
endif
stop
@enduml