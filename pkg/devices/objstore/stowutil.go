package objstore

import (
	"fmt"

	"github.com/graymeta/stow"

	//Load drivers
	"github.com/graymeta/stow/azure"  //Azure storage
	"github.com/graymeta/stow/b2"     //Backblaze storage
	"github.com/graymeta/stow/google" //Google storage
	"github.com/graymeta/stow/local"  //local storage
	"github.com/graymeta/stow/oracle" //oracle storage
	"github.com/graymeta/stow/s3"     //s3 storage
	"github.com/graymeta/stow/sftp"   //sftp storage
	"github.com/graymeta/stow/swift"  //swift storage
)

//The list of all the known ObjectStore (stow.Location) kinds without having
// to import the driver package for each.
const (
	KindAzure               = azure.Kind
	KindBackBlazeB2         = b2.Kind
	KindGoogleCloudStorage  = google.Kind
	KindLocalTest           = local.Kind
	KindS3                  = s3.Kind
	KindOracleObjectStorage = oracle.Kind
	KindSFTP                = sftp.Kind
	KindSwift               = swift.Kind
)

//SupportsMetaData returns false if the provided ObjectStore kind is known to
// not support metadata
func SupportsMetaData(kind string) bool {
	switch kind {
	case KindLocalTest, KindSFTP:
		return false
	}
	return true
}

// NewStore dials stow storage. See stow.Dial for more info
func NewStore(kind string, config stow.Config) (stow.Location, error) {
	return stow.Dial(kind, config)
}

// ValidateConfig verifies config parameters. See stow.Validate for more info
func ValidateConfig(kind string, config stow.Config) error {
	return stow.Validate(kind, config)
}

func describeContainer(container stow.Container) string {
	return fmt.Sprintf("remote container %q (%q)", container.ID(), container.Name())
}

func describeItem(item stow.Item) string {
	return fmt.Sprintf("remote object %q (%q at %q)", item.ID(), item.Name(), item.URL())
}
