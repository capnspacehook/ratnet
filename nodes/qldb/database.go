package qldb

import (
	"database/sql"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/awgh/ratnet/api"
)

var sqlDebug = false

// THIS SHOULD BE THE ONLY FILE THAT INCLUDES database/sql !!!
//		(ok, and qlnode, but only for the Node.db var declaration)

func closeDB(db *sql.DB) {
	_ = db.Close()
}

//
// Generic Database Functions
//

func (node *Node) transactExec(sql string, params ...interface{}) {
	node.mutex.Lock()
	defer node.mutex.Unlock()
	c := node.db()
	defer closeDB(c)

	tx, err := c.Begin()
	if err != nil {
		log.Fatal(sql, params, err.Error())
	}
	if sqlDebug {
		log.Println(sql, params)
	}
	_, err = tx.Exec(sql, params...)
	if err != nil {
		log.Fatal(sql, params, err.Error())
	}
	err = tx.Commit()
	if err != nil {
		log.Fatal(sql, params, err.Error())
	}
}

/*
func (node *Node) transactQuery(sql string, params ...interface{}) *sql.Rows {
	node.mutex.Lock()
	defer node.mutex.Unlock()
	c := node.db()
	defer closeDB(c)

	tx, err := c.Begin()
	if err != nil {
		log.Fatal(err.Error())
	}
	if sqlDebug {
		log.Println(sql, params)
	}
	r, err := tx.Query(sql, params...)
	if err != nil {
		log.Fatal(err.Error())
	}
	err = tx.Commit()
	if err != nil {
		log.Fatal(err.Error())
	}
	return r
}
*/
/*
func (node *Node) transactQueryRow(sqlq string, params ...interface{}) *sql.Row {
	node.mutex.Lock()
	defer node.mutex.Unlock()
	c := node.db()
	defer closeDB(c)
	tx, err := c.Begin()
	if err != nil {
		log.Fatal("transactQueryRow begin fatal: " + err.Error())
	}
	if sqlDebug {
		log.Println(sqlq, params)
	}
	var r *sql.Row
	if params == nil {
		r = tx.QueryRow(sqlq)
	} else {
		r = tx.QueryRow(sqlq, params...)
	}
	err = tx.Commit()
	if err != nil {
		log.Fatal("transactQueryRow commit fatal: " + err.Error())
	}
	return r
}
*/
//
// End Generic Database Functions
//

//
// Specific Database Functions
//

func (node *Node) qlGetContactPubKey(name string) (string, error) {
	c := node.db()
	defer closeDB(c)

	sqlq := "SELECT cpubkey FROM contacts WHERE name==$1;"
	r := c.QueryRow(sqlq, name)
	if sqlDebug {
		log.Println(sqlq, name)
	}
	var pubs string
	if err := r.Scan(&pubs); err == sql.ErrNoRows {
		return "", nil
	} else if err != nil {
		node.errMsg(err, true)
		return "", err
	}
	return pubs, nil
}

func (node *Node) qlGetContacts() ([]api.Contact, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT name,cpubkey FROM contacts;"
	if sqlDebug {
		log.Println(sqlq)
	}
	r, err := c.Query(sqlq)
	if r == nil || err != nil {
		return nil, err
	}
	defer r.Close()

	var contacts []api.Contact
	for r.Next() {
		var d api.Contact
		if err := r.Scan(&d.Name, &d.Pubkey); err != nil {
			return nil, err
		}
		contacts = append(contacts, d)
	}
	return contacts, nil
}

func (node *Node) qlAddContact(name, pubkey string) error {
	c := node.db()
	defer closeDB(c)
	// todo: sanity check key via bencrypt
	tx, err := c.Begin()
	if err != nil {
		return err
	}
	sqlq := "DELETE FROM contacts WHERE name==$1;"
	_, _ = tx.Exec(sqlq, name)
	if sqlDebug {
		log.Println(sqlq)
	}
	sqlq = "INSERT INTO contacts VALUES( $1, $2 );"
	_, err = tx.Exec(sqlq, name, pubkey)
	if sqlDebug {
		log.Println(sqlq)
	}
	if err != nil {
		node.errMsg(err, true)
		return err
	}
	err = tx.Commit()
	if err != nil {
		node.errMsg(err, true)
		return err
	}
	return nil
}

func (node *Node) qlDeleteContact(name string) {
	node.transactExec("DELETE FROM contacts WHERE name==$1;", name)
}

func (node *Node) qlGetChannelPrivKey(name string) (string, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT privkey FROM channels WHERE name==$1;"
	if sqlDebug {
		log.Println(sqlq, name)
	}
	r := c.QueryRow(sqlq, name)
	var privkey string
	if err := r.Scan(&privkey); err == sql.ErrNoRows {
		return "", nil
	} else if err != nil {
		node.errMsg(err, true)
		return "", err
	}
	return privkey, nil
}

func (node *Node) qlGetChannels() ([]api.Channel, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT name,privkey FROM channels;"
	if sqlDebug {
		log.Println(sqlq)
	}
	r, err := c.Query(sqlq)
	if r == nil || err != nil {
		return nil, err
	}
	//defer r.Close()
	var channels []api.Channel
	for r.Next() {
		var n, p string
		if err := r.Scan(&n, &p); err != nil {
			return nil, err
		}
		prv := node.contentKey.Clone()
		if err := prv.FromB64(p); err != nil {
			return nil, err
		}
		channels = append(channels,
			api.Channel{Name: n, Pubkey: prv.GetPubKey().ToB64()})
	}
	return channels, nil
}

func (node *Node) qlGetChannelPrivs() ([]api.ChannelPriv, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT name,privkey FROM channels;"
	if sqlDebug {
		log.Println(sqlq)
	}
	r, err := c.Query(sqlq)
	if r == nil || err != nil {
		return nil, err
	}
	defer r.Close()
	var channels []api.ChannelPriv
	for r.Next() {
		var n, p string
		if err := r.Scan(&n, &p); err != nil {
			return nil, err
		}
		prv := node.contentKey.Clone()
		if err := prv.FromB64(p); err != nil {
			return nil, err
		}
		channels = append(channels,
			api.ChannelPriv{Name: n, Privkey: prv,
				Pubkey: prv.GetPubKey().ToB64()})
	}
	return channels, nil
}

func (node *Node) qlAddChannel(name, privkey string) error {
	c := node.db()
	defer closeDB(c)
	// todo: sanity check key via bencrypt
	tx, err := c.Begin()
	if err != nil {
		return err
	}
	sqlq := "DELETE FROM channels WHERE name==$1;"
	if sqlDebug {
		log.Println(sqlq, name)
	}
	_, _ = tx.Exec(sqlq, name)

	sqlq = "INSERT INTO channels VALUES( $1, $2 );"
	if sqlDebug {
		log.Println(sqlq, name, privkey)
	}
	if _, err := tx.Exec(sqlq, name, privkey); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (node *Node) qlDeleteChannel(name string) {
	node.transactExec("DELETE FROM channels WHERE name==$1;", name)
}

func (node *Node) qlGetProfile(name string) (*api.Profile, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT enabled,privkey FROM profiles WHERE name==$1;"
	if sqlDebug {
		log.Println(sqlq, name)
	}
	r := c.QueryRow(sqlq, name)
	var e bool
	var prv string
	if err := r.Scan(&e, &prv); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	profile := new(api.Profile)
	profile.Enabled = e
	profile.Name = name
	pk := node.contentKey.Clone()
	if err := pk.FromB64(prv); err != nil {
		return nil, err
	}
	profile.Pubkey = pk.GetPubKey().ToB64()
	return profile, nil
}

func (node *Node) qlGetProfiles() ([]api.Profile, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT name,enabled,privkey FROM profiles;"
	if sqlDebug {
		log.Println(sqlq)
	}
	r, err := c.Query(sqlq)
	if r == nil || err != nil {
		return nil, err
	}
	defer r.Close()
	var profiles []api.Profile
	for r.Next() {
		var p api.Profile
		var prv string
		if err := r.Scan(&p.Name, &p.Enabled, &prv); err != nil {
			return nil, err
		}
		pk := node.contentKey.Clone()
		if err := pk.FromB64(prv); err != nil {
			return nil, err
		}
		p.Pubkey = pk.GetPubKey().ToB64()
		profiles = append(profiles, p)
	}
	return profiles, nil
}

func (node *Node) qlAddProfile(name string, enabled bool) error {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT * FROM profiles WHERE name==$1;"
	if sqlDebug {
		log.Println(sqlq, name)
	}
	r := c.QueryRow(sqlq, name)
	var n, key, al string
	if err := r.Scan(&n, &key, &al); err == sql.ErrNoRows {
		// generate new profile keypair
		profileKey := node.contentKey.Clone()
		profileKey.GenerateKey()

		// insert new profile
		node.transactExec("INSERT INTO profiles VALUES( $1, $2, $3 )",
			name, profileKey.ToB64(), enabled)

	} else if err == nil {
		// update profile
		node.transactExec("UPDATE profiles SET enabled=$1 WHERE name==$2;",
			enabled, name)
	} else {
		return err
	}
	return nil
}

func (node *Node) qlDeleteProfile(name string) {
	node.transactExec("DELETE FROM profiles WHERE name==$1;", name)
}

func (node *Node) qlGetProfilePrivateKey(name string) string {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT privkey FROM profiles WHERE name==$1;"
	if sqlDebug {
		log.Println(sqlq, name)
	}
	row := c.QueryRow(sqlq, name)
	var pk string
	if err := row.Scan(&pk); err != nil {
		return ""
	}
	return pk
}

func (node *Node) qlGetPeer(name string) (*api.Peer, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT uri,enabled FROM peers WHERE name==$1;"
	if sqlDebug {
		log.Println(sqlq, name)
	}
	r := c.QueryRow(sqlq, name)
	var u string
	var e bool
	if err := r.Scan(&u, &e); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	peer := new(api.Peer)
	peer.Name = name
	peer.Enabled = e
	peer.URI = u
	return peer, nil
}

func (node *Node) qlGetPeers(group string) ([]api.Peer, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT name,uri,enabled,peergroup FROM peers WHERE peergroup==$1;"
	if sqlDebug {
		log.Println(sqlq, group)
	}
	r, err := c.Query(sqlq, group)
	if r == nil || err != nil {
		return nil, err
	}
	defer r.Close()
	var peers []api.Peer
	for r.Next() {
		var s api.Peer
		if err := r.Scan(&s.Name, &s.URI, &s.Enabled, &s.Group); err != nil {
			return nil, err
		}
		peers = append(peers, s)
	}
	return peers, nil
}

func (node *Node) qlAddPeer(name string, enabled bool, uri string, group string) error {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT name FROM peers WHERE name==$1 AND peergroup==$2;"
	if sqlDebug {
		log.Println(sqlq, name, group)
	}
	r := c.QueryRow(sqlq, name, group)
	var n string
	if err := r.Scan(&n); err == sql.ErrNoRows {
		node.debugMsg("New Server")
		node.transactExec("INSERT INTO peers (name,uri,enabled,peergroup) VALUES( $1, $2, $3, $4 );",
			name, uri, enabled, group)
	} else if err == nil {
		node.debugMsg("Update Server")
		node.transactExec("UPDATE peers SET enabled=$1,uri=$2,peergroup=$3 WHERE name==$4;",
			enabled, uri, group, name)
	} else {
		return err
	}
	return nil
}

func (node *Node) qlDeletePeer(name string) {
	node.transactExec("DELETE FROM peers WHERE name==$1;", name)
}

func (node *Node) qlOutboxEnqueue(channelName string, msg []byte, ts int64, checkExists bool) error {

	doInsert := !checkExists

	if checkExists {
		c := node.db()
		defer closeDB(c)
		// save message in my outbox, if not already present
		sqlq := "SELECT channel FROM outbox WHERE channel==$1 AND msg==$2;"
		if sqlDebug {
			log.Println(sqlq, channelName, msg)
		}
		r1 := c.QueryRow(sqlq, channelName, msg)
		var rc string
		err := r1.Scan(&rc)
		if err == sql.ErrNoRows {
			// we don't have this yet, so add it
			doInsert = true
		} else if err != nil {
			return err
		}
	}
	if doInsert {
		node.transactExec("INSERT INTO outbox(channel,msg,timestamp) VALUES($1,$2,$3);",
			channelName, msg, ts)
	}
	return nil
}

func (node *Node) outboxBulkInsert(channelName string, timestamp int64, msgs [][]byte) {
	c := node.db()
	defer closeDB(c)
	tx, err := c.Begin()
	if err != nil {
		log.Fatal(err.Error())
	}
	args := make([]interface{}, 1+(2*len(msgs)))
	args[0] = channelName
	//args[1] = timestamp
	idx := 2                                                    // starting 1-based index for 2nd arg
	sql := "INSERT INTO outbox(channel, msg, timestamp) VALUES" //($1,$2, $3);
	for i, v := range msgs {
		//sql += "($1,$" + strconv.Itoa(i+3) + ", $2)"
		sql += "($1,$" + strconv.Itoa(idx) + ", $" + strconv.Itoa(idx+1) + ")"
		if i != len(msgs) {
			sql += ", "
		} else {
			sql += ";"
		}
		args[idx-1] = v // convert to 0-based index here
		args[idx] = timestamp
		timestamp++ // increment timestamp by one each message to simplify queueing
		idx += 2
	}
	if sqlDebug {
		log.Println(sql, args)
	}
	_, err = tx.Exec(sql, args...)
	if err != nil {
		log.Fatal(err.Error())
	}
	err = tx.Commit()
	if err != nil {
		log.Fatal(err.Error())
	}
}

func (node *Node) qlGetMessages(lastTime, maxBytes int64, channelNames ...string) ([][]byte, int64, error) {
	c := node.db()
	defer closeDB(c)
	lastTimeReturned := lastTime

	// Build the query

	wildcard := false
	if len(channelNames) < 1 {
		wildcard = true // if no channels are given, get everything
	} else {
		for _, cname := range channelNames {
			for _, char := range cname {
				if !strings.Contains("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0987654321", string(char)) {
					return nil, lastTimeReturned, errors.New("Invalid character in channel name")
				}
			}
		}
	}
	sqlq := "SELECT msg, timestamp FROM outbox"
	if lastTime != 0 {
		sqlq += " WHERE (int64(" + strconv.FormatInt(lastTime, 10) +
			") < timestamp)"
	}
	if !wildcard && len(channelNames) > 0 { // QL is broken?  couldn't make it work with prepared stmts
		if lastTime != 0 {
			sqlq += " AND"
		} else {
			sqlq += " WHERE"
		}
		sqlq = sqlq + " channel IN( \"" + channelNames[0] + "\""
		for i := 1; i < len(channelNames); i++ {
			sqlq = sqlq + ",\"" + channelNames[i] + "\""
		}
		sqlq = sqlq + " )"
	}
	sqlq = sqlq + " ORDER BY timestamp ASC;"

	var msgs [][]byte
	var bytesRead int64

	r, err := c.Query(sqlq)
	if r == nil || err != nil {
		return nil, lastTimeReturned, err
	}
	defer r.Close()

	n := 0
	for r.Next() {
		n++
		var msg []byte
		var ts int64
		r.Scan(&msg, &ts)
		if bytesRead+int64(len(msg)) >= maxBytes { // no room for next msg
			log.Printf("skipping messages after %d results\n", n)
			if n == 0 {
				return nil, lastTimeReturned, errors.New("Result too big to be fetched on this transport! Flush and rechunk")
			}
		}
		if ts > lastTimeReturned {
			lastTimeReturned = ts
		} else {
			log.Printf("Timestamps not increasing - prev: %d  cur: %d\n", lastTimeReturned, ts)
		}
		msgs = append(msgs, msg)
		bytesRead += int64(len(msg))
	}

	return msgs, lastTimeReturned, nil
}

func (node *Node) AddStream(streamID uint32, totalChunks uint32, channelName string) error {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT streamid FROM streams WHERE streamid==$1;"
	if sqlDebug {
		log.Println(sqlq, streamID)
	}
	r := c.QueryRow(sqlq, streamID)
	var n int64
	if err := r.Scan(&n); err == sql.ErrNoRows {
		node.debugMsg("New Stream Header")
		node.transactExec("INSERT INTO streams (streamid,parts,channel) VALUES( $1, $2, $3 );",
			streamID, totalChunks, channelName)
	} else if err == nil {
		node.debugMsg("Update Server")
		node.transactExec("UPDATE streams SET parts=$1,channel=$2 WHERE streamid==$4;",
			totalChunks, channelName, streamID)
	} else {
		return err
	}
	return nil
}

func (node *Node) AddChunk(streamID uint32, chunkNum uint32, data []byte) error {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT chunknum FROM chunks WHERE streamid==$1 AND chunknum==$2;"
	if sqlDebug {
		log.Println(sqlq, streamID, chunkNum)
	}
	r := c.QueryRow(sqlq, streamID, chunkNum)
	var n int64
	if err := r.Scan(&n); err == sql.ErrNoRows {
		node.debugMsg("New Chunk")
		node.transactExec("INSERT INTO chunks (streamid,chunknum,data) VALUES( $1, $2, $3 );",
			streamID, chunkNum, data)
	} else if err == nil {
		node.debugMsg("Update Chunk")
		node.transactExec("UPDATE chunks SET data=$1 WHERE streamid==$2 AND chunknum==$3;",
			data, streamID, chunkNum)
	} else {
		return err
	}
	return nil
}

func (node *Node) qlClearStream(streamID uint32) error {
	node.transactExec("DELETE FROM chunks WHERE streamid == $1;", streamID)
	node.transactExec("DELETE FROM streams WHERE streamid == $1;", streamID)
	return nil
}

func (node *Node) qlGetStreams() ([]api.StreamHeader, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT streamid,parts,channel FROM streams;"
	if sqlDebug {
		log.Println(sqlq)
	}
	r, err := c.Query(sqlq)
	if r == nil || err != nil {
		return nil, err
	}
	defer r.Close()
	var streams []api.StreamHeader
	for r.Next() {
		var s api.StreamHeader
		if err := r.Scan(&s.StreamID, &s.NumChunks, &s.ChannelName); err != nil {
			return nil, err
		}
		streams = append(streams, s)
	}
	return streams, nil
}

func (node *Node) qlGetChunkCount(streamID uint32) (uint64, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT count() FROM chunks WHERE streamid==$1;"
	if sqlDebug {
		log.Println(sqlq, streamID)
	}
	r := c.QueryRow(sqlq, streamID)

	var count int
	if err := r.Scan(&count); err != nil {
		return 0, err
	}
	return uint64(count), nil
}

func (node *Node) qlGetChunks(streamID uint32) ([]api.Chunk, error) {
	c := node.db()
	defer closeDB(c)
	sqlq := "SELECT streamid,chunknum,data FROM chunks WHERE streamid==$1 ORDER BY chunknum ASC;"
	if sqlDebug {
		log.Println(sqlq, streamID)
	}
	r, err := c.Query(sqlq, streamID)
	if r == nil || err != nil {
		return nil, err
	}
	defer r.Close()
	var chunks []api.Chunk
	for r.Next() {
		var s api.Chunk
		if err := r.Scan(&s.StreamID, &s.ChunkNum, &s.Data); err != nil {
			return nil, err
		}
		chunks = append(chunks, s)
	}
	return chunks, nil
}

// FlushOutbox : Deletes outbound messages older than maxAgeSeconds seconds
func (node *Node) FlushOutbox(maxAgeSeconds int64) {
	ts := time.Now().UnixNano()
	ts = ts - (maxAgeSeconds * 1000000000)
	sql := "DELETE FROM outbox WHERE timestamp < ($1);"

	// todo: below does not work on android/arm, investigate
	//sql := "DELETE FROM outbox WHERE since(timestamp) > duration(\"" +
	//	strconv.FormatInt(maxAgeSeconds, 10) + "s\");"
	//log.Println("Flushed Database (seconds): ", maxAgeSeconds)

	node.transactExec(sql, ts)
}

// BootstrapDB - Initialize or open a database file
func (node *Node) BootstrapDB(database string) func() *sql.DB {

	if node.db != nil {
		return node.db
	}

	node.db = func() *sql.DB {
		//log.Println("db: " + database) //todo: why does this trigger so much?
		c, err := sql.Open("ql", database)
		if err != nil {
			node.errMsg(errors.New("DB Error Opening: "+database+" => "+err.Error()), true)
		}
		return c
	}

	// One-time Initialization
	node.transactExec(`
		CREATE TABLE IF NOT EXISTS contacts (
			name	string	NOT NULL,
			cpubkey	string	NOT NULL
		);		
	`)
	//CREATE UNIQUE INDEX IF NOT EXISTS contactid ON contacts (id());

	node.transactExec(`
		CREATE TABLE IF NOT EXISTS channels ( 			
			name	string	NOT NULL,
			privkey	string	NOT NULL
		);
	`)

	node.transactExec(`
		CREATE TABLE IF NOT EXISTS config ( 
			name	string	NOT NULL,
			value	string	NOT NULL
		);
	`)

	/*  timestamp field must stay int64 and not time type,
	due to a unknown bug only on android/arm in cznic/ql via sql driver
	*/
	node.transactExec(`
		CREATE TABLE IF NOT EXISTS outbox (
			channel		string	DEFAULT "",
			msg			blob	NOT NULL,
			timestamp	int64	NOT NULL
		);
	`)
	node.transactExec(`
			CREATE INDEX IF NOT EXISTS outboxID ON outbox (timestamp);
	`)

	node.transactExec(`
		CREATE TABLE IF NOT EXISTS peers (
			name	string	NOT NULL,  
			uri			string	NOT NULL,
			enabled		bool	NOT NULL,
			peergroup   string  NOT NULL,
			pubkey	string	DEFAULT NULL
		);
	`)

	node.transactExec(`
		CREATE TABLE IF NOT EXISTS profiles (
			name	string	NOT NULL,
			privkey	string	NOT NULL,
			enabled	bool	NOT NULL
		);
	`)

	node.transactExec(`
	CREATE TABLE IF NOT EXISTS chunks (		
		streamid	int64	NOT NULL,
		chunknum	int64	NOT NULL,
		data		blob	NOT NULL
	);
	`)

	node.transactExec(`
	CREATE TABLE IF NOT EXISTS streams (		
		streamid		int64	NOT NULL,
		parts			int64	NOT NULL,
		channel			string	NOT NULL
	);
	`)

	var n, s string
	c := node.db()
	defer closeDB(c)

	// Content Key Setup
	// todo: content key needs to go away and be replaced by vectorized enabled profiles.
	r1 := c.QueryRow("SELECT * FROM config WHERE name == `contentkey`;")
	err := r1.Scan(&n, &s)
	if err == sql.ErrNoRows {
		node.contentKey.GenerateKey()
		bs := node.contentKey.ToB64()
		node.transactExec("INSERT INTO config VALUES( `contentkey`, $1 );", bs)
	} else if err != nil {
		node.errMsg(err, true)
	} else {
		err = node.contentKey.FromB64(s)
		if err != nil {
			node.errMsg(err, true)
		}
	}
	// Routing Key Setup
	r2 := c.QueryRow("SELECT * FROM config WHERE name == `routingkey`;")
	if err := r2.Scan(&n, &s); err == sql.ErrNoRows {
		node.routingKey.GenerateKey()
		bs := node.routingKey.ToB64()
		node.transactExec("INSERT INTO config VALUES( `routingkey`, $1 );", bs)
	} else if err != nil {
		node.errMsg(err, true)
	} else {
		err = node.routingKey.FromB64(s)
		if err != nil {
			node.errMsg(err, true)
		}
	}
	node.refreshChannels()
	return node.db
}
