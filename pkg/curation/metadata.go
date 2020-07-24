package curation

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"time"

	"github.com/anchore/go-version"
	"github.com/anchore/grype-db/internal/file"
	"github.com/anchore/grype-db/internal/log"
	"github.com/spf13/afero"
)

const MetadataFileName = "metadata.json"

// Metadata represents the basic identifying information of a database flat file (built/version) and a way to
// verify the contents (checksum).
type Metadata struct {
	Built    time.Time
	Version  *version.Version
	Checksum string
}

// MetadataJSON is a helper struct for parsing and assembling Metadata objects to and from JSON.
type MetadataJSON struct {
	Built    string `json:"built"` // RFC 3339
	Version  string `json:"version"`
	Checksum string `json:"checksum"`
}

// ToMetadata converts a MetadataJSON object to a Metadata object.
func (m MetadataJSON) ToMetadata() (Metadata, error) {
	build, err := time.Parse(time.RFC3339, m.Built)
	if err != nil {
		return Metadata{}, fmt.Errorf("cannot convert built time (%s): %+v", m.Built, err)
	}

	ver, err := version.NewVersion(m.Version)
	if err != nil {
		return Metadata{}, fmt.Errorf("cannot parse version (%s): %+v", m.Version, err)
	}

	metadata := Metadata{
		Built:    build.UTC(),
		Version:  ver,
		Checksum: m.Checksum,
	}

	return metadata, nil
}

func metadataPath(dir string) string {
	return path.Join(dir, MetadataFileName)
}

// NewMetadataFromDir generates a Metadata object from a directory containing a vulnerability.db flat file.
func NewMetadataFromDir(fs afero.Fs, dir string) (*Metadata, error) {
	metadataFilePath := metadataPath(dir)
	if !file.Exists(fs, metadataFilePath) {
		return nil, nil
	}
	f, err := fs.Open(metadataFilePath)
	if err != nil {
		return nil, fmt.Errorf("unable to open DB metadata path (%s): %w", metadataFilePath, err)
	}
	defer f.Close()

	var m Metadata
	err = json.NewDecoder(f).Decode(&m)
	if err != nil {
		return nil, fmt.Errorf("unable to parse DB metadata (%s): %w", metadataFilePath, err)
	}
	return &m, nil
}

func (m *Metadata) UnmarshalJSON(data []byte) error {
	var mj MetadataJSON
	if err := json.Unmarshal(data, &mj); err != nil {
		return err
	}
	me, err := mj.ToMetadata()
	if err != nil {
		return err
	}
	*m = me
	return nil
}

// IsSupercededBy takes a ListingEntry and determines if the entry candidate is newer than what is hinted at
// in the current Metadata object.
func (m *Metadata) IsSupercededBy(entry *ListingEntry) bool {
	if m == nil {
		log.Debugf("cannot find existing metadata, using update...")
		// any valid update beats no database, use it!
		return true
	}

	if entry.Version.GreaterThan(m.Version) {
		log.Debugf("update is a newer version than the current database, using update...")
		// the listing is newer than the existing db, use it!
		return true
	}

	if entry.Built.After(m.Built) {
		log.Debugf("existing database (%s) is older than candidate update (%s), using update...", m.Built.String(), entry.Built.String())
		// the listing is newer than the existing db, use it!
		return true
	}

	log.Debugf("existing database is already up to date")
	return false
}

func (m Metadata) String() string {
	return fmt.Sprintf("Metadata(built=%s version=%s checksum=%s)", m.Built, m.Version, m.Checksum)
}

// Write out a Metadata object to the given path.
func (m Metadata) Write(toPath string) error {
	metadata := MetadataJSON{
		Built:    m.Built.UTC().Format(time.RFC3339),
		Version:  m.Version.String(),
		Checksum: m.Checksum,
	}

	contents, err := json.MarshalIndent(&metadata, "", " ")
	if err != nil {
		return fmt.Errorf("failed to encode metadata file: %w", err)
	}

	err = ioutil.WriteFile(toPath, contents, 0600)
	if err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}
	return nil
}