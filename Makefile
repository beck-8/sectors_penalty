SHELL=/usr/bin/env bash


# 指定你的 Go 项目的包名
PACKAGE_NAME=sectors_penalty

# 指定你的 Go 编译器
GOCC=go

# 定义所有的目标
all: deps build

# 构建二进制文件
build:
	$(GOCC) build -ldflags "-X 'main.CurrentCommit=`git show -s --format=%H|cut -b 1-10`'" -o $(PACKAGE_NAME)

# 安装依赖
deps:
	touch $@
	git submodule update --init --recursive
	@$(MAKE) -C extern/filecoin-ffi


# 清理临时文件和二进制文件
clean:
	rm -f $(PACKAGE_NAME)
	rm -r deps
	@$(MAKE) -C extern/filecoin-ffi clean
