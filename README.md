# Webrtc Demo

## 概述

分为服务端模块和android客户端模块
其中AndroidClient为安卓客户端
其余为基于pion的服务端模块

## peer-connection

建立连接的示例

## play-from-server

服务端播放本地音频，android客户端播放 (使用WriteSample接口)

# play-from-server-rtp

服务端播放本地音频，使用WriteRTP接口

## webrtc-server-socket

websocket支持offer/answer/candidate完整的信令
支持从客户端或者服务端任意一端发起协商
支持在连接建立后添加新的track(此时需要重新协商)

## socket-remove-track

删除音频轨道

## multiple-track

同时添加多个音频轨道并播放

## negotiate-data-channel

服务端重新协商通过DataChannel发起

## custom-rtp-extension

使用默认的扩展，写入自定义数据
