# CNSoftwareCup-AAF

An Algorithm Access Framework for ChinaSoftwareCup-2020

## 获奖情况

1. [中软杯国家级一等奖](http://www.cnsoftbei.com/plus/view.php?aid=565)
  赛题：[企业异构数据集成及应用平台](http://www.cnsoftbei.com/plus/view.php?aid=530)  
  团队信息(编号+名称)：19665 我还能肝  
  成员：汤家平、殷燚涛、江凯  
  指导老师：	陈慧萍

## 系统整体架构描述

后端通过TLS连接到算法接入框架，一共维持两个连接，一个连接用于发送命令，一个连接用于向后端发送运行结果。对于算法进程的运行，本框架采用docker容器实现算法进程的隔离运行。

![整体架构图](https://raw.githubusercontent.com/yin1999/CNSoftwareCup-AAF/master/img/%E6%95%B4%E4%BD%93%E6%9E%B6%E6%9E%84.svg)

## 写在最后

因时间仓促，且要在边学习边应用的情况下实现算法的接入和运行，框架仅仅实现了需要的功能，整体结构可能略有混乱。

该repo仅作参考、记录自己的比赛经历之用
