# sectors_penalty

## install && run
> 返回慢时，优化此程序到lotus rpc之间的链接
```bash
git clone https://github.com/beck-8/sectors_penalty.git
make build

# run
./sectors_penalty

# use port 6666
./sectors_penalty -port 6666

# use other lotus rpc
export FULLNODE_RPC="https://api.node.glif.io/rpc/v0"
./sectors_penalty
```
## Usage
> miner 节点ID  
all 是否展示全部的扇区（包含过期的）  
offset 往前/往后推移多少天（+20/-20）  
#### 查看f01155的信息  
```
http://127.0.0.1:8099/penalty?miner=f01155
```
#### 查看f01155全部的信息（包含已经过期的）
```
http://127.0.0.1:8099/penalty?miner=f01155&all=1
```
#### 查看f01155 20天后的终结惩罚信息
```
http://127.0.0.1:8099/penalty?miner=f01155&offset=20
```

