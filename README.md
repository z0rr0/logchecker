# LogChecker

It is a simple library to check a list of logs files and send notification about their abnormal activities.

### Usage

```go
import "logchecker"
import "log"
// ...

logger := logchecker.New()
if err := logchecker.InitConfig(logger, "config.json"); err != nil {
    log.Panicf("logchecker error: %v\n", err)
}
```

### Configuration

Files for observation can be added using a configuration file, see examples in [config.example.json](https://github.com/z0rr0/logchecker/blob/master/config.example.json).

```javascript
{
  "sender": {                                   // send email through this smtp server
    "user": "user@host.com",                    // username
    "password": "password",                     // user password
    "host": "smtp.host.com",                    // smtp host name
    "addr": "smtp.host.com:25"                  // server address + port
  },
  "observed": [                                 // array of observed services
    {
      "name": "Nginx",                          // service name
      "files": [                                // array of files for observation
        {
          "file": "/var/log/nginx/error.log",   // file for observation
          "delay": 180,                         // delay between checks
          "pattern": "ERROR",                   // to account lines with the pattern
          "boundary": 1,                        // send email if sum greater than boundary value
          "increasing": true,                   // increase boundary value as 2^n
          "emails": ["user_1@host.com"],        // send email to these users
          "limist": [10, 20, 100]               // emails' limits: per hour / day / week
        }
      ]
    }
  ]
}
```

### Testing

Use standard Go testing mechanism:

```bash
cd $GOPATH/src/github.com/z0rr0/logchecker
go test
```

There is a [nice article](http://blog.golang.org/cover) about tests covering.

### License

This source code is governed by a [LGPLv3](https://www.gnu.org/licenses/lgpl-3.0.txt) license that can be found in the [LICENSE](https://github.com/z0rr0/logchecker/blob/master/LICENSE) file.

<img src="https://www.gnu.org/graphics/lgplv3-147x51.png" title="LGPLv3 logo">
