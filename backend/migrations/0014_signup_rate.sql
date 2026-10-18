-- 0014: 设备注册 IP 限流（防批量造号薅免费额度；邮箱登录不受影响）
CREATE TABLE signup_rate (
    ip    VARCHAR(45) NOT NULL,
    day   DATE NOT NULL,
    count INT NOT NULL DEFAULT 1,
    PRIMARY KEY (ip, day)
);
