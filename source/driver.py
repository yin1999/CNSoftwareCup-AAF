import socket
import sys
import json
import time

Msql = 'mysql'
SQL = 'sqlserver'
Influxdb = 'influxdb'

_remoteAddr = "172.17.0.1"
_remotePort = 2076
_s = object
_init = False
_args = object
_zero = '\x00'.encode()
_statusOK = 'ok'
_step = 2048

class DBInfo:
    def __init__(self):
        self.Type = ''
        self.Addr = ''
        self.Database = ''
        self.UserName = ''
        self.Password = ''

def __init__():
    global _init
    if _init:
        return
    global _s
    global _args
    token = sys.argv[1]
    _args = sys.argv[2:]
    _s = socket.socket(socket.AF_INET,socket.SOCK_STREAM)
    _s.connect((_remoteAddr, _remotePort))
    _s.send((token+"\0").encode())
    if _receive() != _statusOK:
        _s.close()
        sys.exit(-1)
    _init = True

def send(data: str) -> int:
    __init__()
    global _s
    length = len(data)
    l = _int32Encoder(length)
    _s.send("send\0".encode())
    data = data.encode()
    if _receive() == "ok":
        _s.send(l)
        i = _step
        while i <= length:
            _s.send(data[i-_step:i])
            time.sleep(0.05)
        if length% _step != 0:
            _s.send(data[i-_step:])
        if _receive() == "ok":
            return 0
    return -1

def Args():
    __init__()
    return _args

def _receive() -> str:
    global _s
    tmp = ''
    c = _s.recv(1)
    while c != _zero:
        tmp += str(c, encoding='utf-8')
        c = _s.recv(1)
    
    return tmp
    
# return a list of DBInfo which contains Type(db type), Addr(db address), UserName(db username), Password(db password), Database(db database)
def getDBList() ->list:
    global _s
    _s.send("dbList\0".encode())
    data = _receive()
    out = json.loads(data)
    L = []
    for v in out:
        t = DBInfo()
        t.Type = v['type']
        t.Addr = v['addr']
        t.Database = v['database']
        t.UserName = v['username']
        t.Password = v['password']
        L.append(t)
    return L

def _int32Encoder(num: int) -> bytes:
    l = [0,0,0,0]
    i = 3
    while num != 0:
        l[i] = num & 0xFF
        num >>= 8
        i -= 1
    return bytes(l)