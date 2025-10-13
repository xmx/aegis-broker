import config from 'aegis/agent/config'
import console from 'console'

const cfg = config.get()
cfg.protocols = ['udp', 'tcp']
if (cfg.addresses.length === 0) {
    cfg.addresses = [
        'broker.example.com:9443'
    ]
}

cfg.protocols.forEach(p => {
    if (p !== 'udp' && p !== 'tcp') {
        throw new Error(`协议 ${p} 填写错误`)
    }
})

console.info('配置设置完毕')
