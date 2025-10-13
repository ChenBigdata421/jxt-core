@echo off
cd /d %~dp0
go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 20m

