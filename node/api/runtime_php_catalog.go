// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import ()

// buildPHPExtensionCatalog 构建 PHP 扩展、检测名称、文件和兼容版本目录。
func buildPHPExtensionCatalog() []phpExtensionDefinition {
	all := []string{"56", "70", "71", "72", "73", "74", "80", "81", "82", "83", "84", "85"}
	from70 := []string{"70", "71", "72", "73", "74", "80", "81", "82", "83", "84", "85"}
	until74 := []string{"56", "70", "71", "72", "73", "74"}
	item := func(name, check, file string, versions []string) phpExtensionDefinition {
		return phpExtensionDefinition{Name: name, Check: check, File: file, Versions: append([]string(nil), versions...)}
	}
	return []phpExtensionDefinition{
		item("amqp", "amqp", "amqp.so", all),
		item("apcu", "apcu", "apcu.so", all),
		item("bcmath", "bcmath", "bcmath.so", all),
		item("ionCube", "ionCube Loader", "ioncube_loader.so", []string{"56", "70", "71", "72", "73", "74", "81", "82"}),
		item("opcache", "Zend OPcache", "opcache.so", all),
		item("memcache", "memcache", "memcache.so", []string{"56", "70", "71", "72", "73", "74", "80"}),
		item("memcached", "memcached", "memcached.so", all),
		item("redis", "redis", "redis.so", all),
		item("mcrypt", "mcrypt", "mcrypt.so", from70),
		item("imagick", "imagick", "imagick.so", all),
		item("xdebug", "xdebug", "xdebug.so", all),
		item("imap", "imap", "imap.so", all),
		item("exif", "exif", "exif.so", all),
		item("intl", "intl", "intl.so", all),
		item("xsl", "xsl", "xsl.so", []string{"56", "70", "71", "72", "73", "74", "80", "81", "82"}),
		item("swoole", "swoole", "swoole.so", all),
		item("zstd", "zstd", "zstd.so", all),
		item("xlswriter", "xlswriter", "xlswriter.so", from70),
		item("oci8", "oci8", "oci8.so", from70),
		item("pdo_oci", "pdo_oci", "pdo_oci.so", from70),
		item("pdo_sqlsrv", "pdo_sqlsrv", "pdo_sqlsrv.so", from70),
		item("sqlsrv", "sqlsrv", "sqlsrv.so", []string{"81", "82", "83", "84"}),
		item("yaf", "yaf", "yaf.so", all),
		item("mongodb", "mongodb", "mongodb.so", all),
		item("yac", "yac", "yac.so", from70),
		item("pgsql", "pgsql", "pgsql.so", all),
		item("ssh2", "ssh2", "ssh2.so", all),
		item("grpc", "grpc", "grpc.so", all),
		item("xhprof", "xhprof", "xhprof.so", all),
		item("protobuf", "protobuf", "protobuf.so", all),
		item("pdo_pgsql", "pdo_pgsql", "pdo_pgsql.so", all),
		item("snmp", "snmp", "snmp.so", all),
		item("ldap", "ldap", "ldap.so", all),
		item("recode", "recode", "recode.so", []string{"56", "70", "71", "72", "73"}),
		item("enchant", "enchant", "enchant.so", all),
		item("pspell", "pspell", "pspell.so", all),
		item("bz2", "bz2", "bz2.so", all),
		item("sysvshm", "sysvshm", "sysvshm.so", all),
		item("calendar", "calendar", "calendar.so", all),
		item("gmp", "gmp", "gmp.so", all),
		item("wddx", "wddx", "wddx.so", until74),
		item("sysvmsg", "sysvmsg", "sysvmsg.so", all),
		item("igbinary", "igbinary", "igbinary.so", all),
		item("zmq", "zmq", "zmq.so", all),
		item("smbclient", "smbclient", "smbclient.so", all),
		item("event", "event", "event.so", all),
		item("mailparse", "mailparse", "mailparse.so", all),
		item("yaml", "yaml", "yaml.so", all),
		item("sg16", "SourceGuardian", "sourceguardian.so", all),
		item("mysqli", "mysqli", "mysqli.so", all),
		item("pdo_mysql", "pdo_mysql", "pdo_mysql.so", all),
		item("zip", "zip", "zip.so", all),
		item("shmop", "shmop", "shmop.so", all),
		item("gd", "gd", "gd.so", all),
		item("pcntl", "pcntl", "pcntl.so", all),
		item("sodium", "sodium", "sodium.so", from70),
		item("gettext", "gettext", "gettext.so", all),
		item("soap", "soap", "soap.so", all),
		item("sysvsem", "sysvsem", "sysvsem.so", all),
		item("sockets", "sockets", "sockets.so", all),
		item("xmlrpc", "xmlrpc", "xmlrpc.so", all),
		item("lz4", "lz4", "lz4.so", all),
		item("msgpack", "msgpack", "msgpack.so", all),
	}
}

// toMap 将 PHP 扩展定义转换为前端扩展目录响应。
func (definition phpExtensionDefinition) toMap(installed bool) map[string]any {
	return map[string]any{"name": definition.Name, "description": definition.Description, "installed": installed, "check": definition.Check, "file": definition.File, "versions": definition.Versions}
}
