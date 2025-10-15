#!/bin/bash

set -e

echo "��� 开始迁移第二批测试文件..."

# 文件列表
FILES=(
    "json_config_test.go"
    "config_conversion_test.go"
)

SRC_DIR="../../../sdk/pkg/eventbus"
DEST_DIR="."

for FILE in "${FILES[@]}"; do
    echo ""
    echo "��� 迁移文件: $FILE"
    
    # 1. 复制文件
    cp "$SRC_DIR/$FILE" "$DEST_DIR/$FILE"
    echo "  ✅ 文件已复制"
    
    # 2. 修改包名
    sed -i 's/^package eventbus$/package function_tests/' "$FILE"
    echo "  ✅ 包名已修改"
    
    # 3. 添加导入 (在第一个import之后)
    sed -i '/^import (/a\"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"' "$FILE"
    echo "  ✅ 导入已添加"
    
    # 4. 更新类型引用
    sed -i 's/\bEventBusConfig\b/eventbus.EventBusConfig/g' "$FILE"
    sed -i 's/\bKafkaConfig\b/eventbus.KafkaConfig/g' "$FILE"
    sed -i 's/\bProducerConfig\b/eventbus.ProducerConfig/g' "$FILE"
    sed -i 's/\bConsumerConfig\b/eventbus.ConsumerConfig/g' "$FILE"
    sed -i 's/\bNATSConfig\b/eventbus.NATSConfig/g' "$FILE"
    sed -i 's/\bRawMessage\b/eventbus.RawMessage/g' "$FILE"
    sed -i 's/\bMarshal\b/eventbus.Marshal/g' "$FILE"
    sed -i 's/\bUnmarshal\b/eventbus.Unmarshal/g' "$FILE"
    sed -i 's/\bMarshalToString\b/eventbus.MarshalToString/g' "$FILE"
    sed -i 's/\bUnmarshalFromString\b/eventbus.UnmarshalFromString/g' "$FILE"
    sed -i 's/\bMarshalFast\b/eventbus.MarshalFast/g' "$FILE"
    sed -i 's/\bUnmarshalFast\b/eventbus.UnmarshalFast/g' "$FILE"
    sed -i 's/\bJSON\b/eventbus.JSON/g' "$FILE"
    sed -i 's/\bJSONFast\b/eventbus.JSONFast/g' "$FILE"
    sed -i 's/\bJSONDefault\b/eventbus.JSONDefault/g' "$FILE"
    sed -i 's/\bNewKafkaEventBus\b/eventbus.NewKafkaEventBus/g' "$FILE"
    
    # 5. 修复双重前缀
    sed -i 's/eventbus\.eventbus\./eventbus./g' "$FILE"
    
    echo "  ✅ 类型引用已更新"
    
    echo "✅ $FILE 迁移完成"
done

echo ""
echo "��� 第二批迁移完成！"
echo "��� 迁移文件数: ${#FILES[@]}"

