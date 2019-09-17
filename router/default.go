package router

import (
	"bytes"
	"encoding/json"
	"errors"
	"hash/crc32"
	"math"
	"sort"
	"sync"

	"github.com/awgh/ratnet"
	"github.com/awgh/ratnet/api"
)

const (
	cacheSize     = 4 * 1024
	cacheDiscount = int(0.25 * cacheSize)
	nonceSize     = 32
)

// RecentBuffer - Used for tracking recently seen messages
type RecentBuffer struct {
	mtx           sync.Mutex
	recentBuffer  map[uint32]int
	reverseBuffer map[int]uint32
	counter       int
}

func newRecentBuffer() (r RecentBuffer) {
	r.recentBuffer = make(map[uint32]int, cacheSize)
	r.reverseBuffer = make(map[int]uint32, cacheSize)
	r.counter = 0
	return
}

// SeenRecently : Returns whether this message should be filtered out by loop detection
func (r *RecentBuffer) SeenRecently(nonce []byte) bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	nonceHash := crc32.ChecksumIEEE(nonce)

	_, seen := r.recentBuffer[nonceHash]
	if !seen {
		r.recentBuffer[nonceHash] = r.counter
		r.reverseBuffer[r.counter] = nonceHash
		if r.counter == math.MaxInt32 { // todo doc: cacheSize is limited to MaxInt32
			r.counter = 0
		} else {
			r.counter++
		}
	}
	// garbage collection
	m := len(r.recentBuffer)
	if m >= cacheSize {
		values := make([]int, 0, m)
		for _, v := range r.recentBuffer {
			values = append(values, v)
		}
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		discount := (cacheSize - m) + cacheDiscount
		for i := 0; i < discount; i++ {
			nh := r.reverseBuffer[values[i]]
			delete(r.reverseBuffer, values[i])
			delete(r.recentBuffer, nh)
		}
	}
	return seen
}

// DefaultRouter - The Default router makes no changes at all,
//                 every message is sent out on the same channel it came in on,
//                 and non-channel messages are consumed but not forwarded
type DefaultRouter struct {
	// Internal
	RecentBuffer

	Patches []api.Patch

	// Configuration Settings

	// CheckContent - Check if incoming messages are for the contentKey
	CheckContent bool
	// CheckChannels - Check if incoming messages are for any of the channel keys
	CheckChannels bool
	// CheckProfiles - Check if incoming messages are for any of the profile keys
	CheckProfiles bool

	// ForwardConsumedContent - Should node forward consumed messages that matched contentKey
	ForwardConsumedContent bool
	// ForwardConsumedContent - Should node forward consumed messages that matched a channel key
	ForwardConsumedChannels bool
	// ForwardConsumedProfile - Should node forward consumed messages that matched a profile key
	ForwardConsumedProfiles bool

	// ForwardUnknownContent - Should node forward non-consumed messages that matched contentKey
	ForwardUnknownContent bool
	// ForwardUnknownContent - Should node forward non-consumed messages that matched a channel key
	ForwardUnknownChannels bool
	// ForwardUnknownProfile - Should node forward non-consumed messages that matched a profile key
	ForwardUnknownProfiles bool
}

func init() {
	ratnet.Routers["default"] = NewRouterFromMap // register this module by name (for deserialization support)
}

// NewRouterFromMap : Makes a new instance of this module from a map of arguments (for deserialization support)
func NewRouterFromMap(r map[string]interface{}) api.Router {
	return NewDefaultRouter()
}

// NewDefaultRouter - returns a new instance of DefaultRouter
func NewDefaultRouter() *DefaultRouter {
	r := new(DefaultRouter)
	r.CheckContent = true
	r.CheckChannels = true
	r.CheckProfiles = false
	r.ForwardUnknownContent = true
	r.ForwardUnknownChannels = true
	r.ForwardUnknownProfiles = false
	r.ForwardConsumedContent = false
	r.ForwardConsumedChannels = true
	r.ForwardConsumedProfiles = false
	// init page maps
	r.RecentBuffer = newRecentBuffer()
	return r
}

// Patch : Redirect messages from one input to different outputs
func (r *DefaultRouter) Patch(patch api.Patch) {
	r.Patches = append(r.Patches, patch)
}

// GetPatches : Returns an array with the mappings of incoming channels to destination channels
func (r *DefaultRouter) GetPatches() []api.Patch {
	return r.Patches
}

func (r *DefaultRouter) forward(node api.Node, msg api.Msg) error {
	for _, p := range r.Patches { //todo: this could be constant-time
		if msg.Name == p.From { // we don't check for IsChan here, we allow forwarding from "" chan to channels
			for i := 0; i < len(p.To); i++ {
				msg.Name = p.To[i]
				if msg.Name == "" {
					msg.IsChan = false
				} else {
					msg.IsChan = true
				}
				if err := node.Forward(msg); err != nil {
					return err
				}
			}
			return nil
		}
	}
	if err := node.Forward(msg); err != nil {
		return err
	}
	return nil
}

// Route - Router that does default behavior
func (r *DefaultRouter) Route(node api.Node, message []byte) error {

	//  Stuff Everything will need just about every time...
	//
	var msg api.Msg
	flags := message[0]
	idx := 1
	msg.IsChan = ((flags & api.ChannelFlag) != 0)
	msg.Chunked = ((flags & api.ChunkedFlag) != 0)
	msg.StreamHeader = ((flags & api.StreamHeaderFlag) != 0)
	var channelLen uint16 // beginning uint16 of message is channel name length
	if msg.IsChan {
		channelLen = (uint16(message[1]) << 8) | uint16(message[2])
		msg.Name = string(message[3 : 3+channelLen]) // flags[0], chan name length[1,2]
		idx += 2 + int(channelLen)                   // skip over the channel name
	}
	if idx+16 >= len(message) {
		return errors.New("Malformed message")
	}
	nonce := message[idx : idx+nonceSize]
	if r.SeenRecently(nonce) { // LOOP PREVENTION before handling or forwarding
		return nil
	}
	cid, err := node.CID() // we need this for cloning
	if err != nil {
		return err
	}
	msg.Content = bytes.NewBuffer(message[idx:])

	// Routing Logic
	if msg.IsChan { // channel message
		consumed := false
		if r.CheckChannels {
			chn, err := node.GetChannel(msg.Name)
			if chn != nil && err == nil { // this is a channel key we know
				pubkey := cid.Clone()
				pubkey.FromB64(chn.Pubkey)
				consumed, err = node.Handle(msg)
				if err != nil {
					return err
				}
			}
		}
		if (!consumed && r.ForwardUnknownChannels) || (consumed && r.ForwardConsumedChannels) {
			if err := r.forward(node, msg); err != nil {
				return err
			}
		}
	} else { // private message (zero length channel)
		// content key case (to be removed, deprecated)
		consumed := false
		if r.CheckContent {
			consumed, err = node.Handle(msg)
			if err != nil {
				return err
			}
		}
		if (!consumed && r.ForwardUnknownContent) || (consumed && r.ForwardConsumedContent) {
			if err := r.forward(node, msg); err != nil {
				return err
			}
		}

		// profile keys case
		consumed = false
		if r.CheckProfiles {
			profiles, err := node.GetProfiles()
			if err != nil {
				return err
			}
			for _, profile := range profiles {
				if !profile.Enabled {
					continue
				}
				pubkey := cid.Clone()
				pubkey.FromB64(profile.Pubkey)
				consumed, err = node.Handle(msg)
				if err != nil {
					return err
				}
				if consumed {
					break
				}
			}
		}
		if (!consumed && r.ForwardUnknownProfiles) || (consumed && r.ForwardConsumedProfiles) {
			if err := r.forward(node, msg); err != nil {
				return err
			}
		}
	}
	return nil
}

// MarshalJSON : Create a serialized JSON blob out of the config of this router
func (r *DefaultRouter) MarshalJSON() (b []byte, e error) {

	return json.Marshal(map[string]interface{}{
		"Router":                  "default",
		"CheckContent":            r.CheckContent,
		"ForwardConsumedContent":  r.ForwardConsumedContent,
		"ForwardUnknownContent":   r.ForwardUnknownContent,
		"CheckProfiles":           r.CheckProfiles,
		"ForwardConsumedProfiles": r.ForwardConsumedProfiles,
		"ForwardUnknownProfiles":  r.ForwardUnknownProfiles,
		"CheckChannels":           r.CheckChannels,
		"ForwardConsumedChannels": r.ForwardConsumedChannels,
		"ForwardUnknownChannels":  r.ForwardUnknownChannels,
		"Patches":                 r.Patches})
}
