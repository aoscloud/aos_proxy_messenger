// SPDX-License-Identifier: Apache-2.0
//
// Copyright (C) 2023 Renesas Electronics Corporation.
// Copyright (C) 2023 EPAM Systems, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vchanmanager_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aoscloud/aos_common/aoserrors"
	pb "github.com/aoscloud/aos_common/api/servicemanager/v3"
	"github.com/aoscloud/aos_messageproxy/vchanmanager"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

/***********************************************************************************************************************
 * Consts
 **********************************************************************************************************************/

const Kilobyte = uint64(1 << 10)

/***********************************************************************************************************************
 * Types
 **********************************************************************************************************************/

type testDownloader struct {
	downloadedFile string
}

type testUnpacker struct {
	filePath string
}

type testVChan struct {
	send chan []byte
	recv chan []byte
}

/***********************************************************************************************************************
 * Init
 **********************************************************************************************************************/

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp: false,
		TimestampFormat:  "2006-01-02 15:04:05.000",
		FullTimestamp:    true,
	})
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)
}

/***********************************************************************************************************************
 * Tests
 **********************************************************************************************************************/

func TestPrivateReadWriteVchan(t *testing.T) {
	tVchanPriv := &testVChan{
		send: make(chan []byte, 1),
		recv: make(chan []byte, 1),
	}

	tVchanPub := &testVChan{
		send: make(chan []byte, 1),
		recv: make(chan []byte, 1),
	}

	vch, err := vchanmanager.New(nil, nil, tVchanPub, tVchanPriv)
	if err != nil {
		t.Errorf("Can't create a new vchannel manager: %v", err)
	}
	defer vch.Close()

	tCases := []struct {
		name string
		data *pb.SMIncomingMessages
	}{
		{
			name: "Test 1",
			data: &pb.SMIncomingMessages{
				SMIncomingMessage: &pb.SMIncomingMessages_SetUnitConfig{
					SetUnitConfig: &pb.SetUnitConfig{
						UnitConfig:    "UnitConfig1",
						VendorVersion: "VendorVersion1",
					},
				},
			},
		},
		{
			name: "Test 2",
			data: &pb.SMIncomingMessages{
				SMIncomingMessage: &pb.SMIncomingMessages_SetUnitConfig{
					SetUnitConfig: &pb.SetUnitConfig{
						UnitConfig:    "UnitConfig1",
						VendorVersion: "VendorVersion1",
					},
				},
			},
		},
	}

	for _, tCase := range tCases {
		t.Run(tCase.name, func(t *testing.T) {
			data, err := proto.Marshal(tCase.data)
			if err != nil {
				t.Errorf("Can't marshal data: %v", err)
			}

			vch.GetSendingChannel() <- data

			select {
			case receivedData := <-tVchanPriv.send:
				incomingData := &pb.SMIncomingMessages{}
				if err := proto.Unmarshal(receivedData, incomingData); err != nil {
					t.Errorf("Can't unmarshal data: %v", err)
				}

				if !proto.Equal(tCase.data, incomingData) {
					t.Errorf("Expected data: %s, received data: %s", tCase.data, incomingData)
				}

			case <-tVchanPub.send:
				t.Errorf("Unexpected data")

			case <-time.After(1 * time.Second):
				t.Errorf("Timeout")
			}
		})
	}

	tCasesOutgoing := []struct {
		name string
		data *pb.SMOutgoingMessages
	}{
		{
			name: "Test 1",
			data: &pb.SMOutgoingMessages{
				SMOutgoingMessage: &pb.SMOutgoingMessages_NodeConfiguration{
					NodeConfiguration: &pb.NodeConfiguration{
						NodeId:   "NodeId1",
						NodeType: "NodeType1",
						NumCpus:  1,
						TotalRam: 1,
					},
				},
			},
		},
		{
			name: "Test 2",
			data: &pb.SMOutgoingMessages{
				SMOutgoingMessage: &pb.SMOutgoingMessages_NodeConfiguration{
					NodeConfiguration: &pb.NodeConfiguration{
						NodeId:   "NodeId2",
						NodeType: "NodeType2",
						NumCpus:  2,
						TotalRam: 23,
					},
				},
			},
		},
	}

	for _, tCase := range tCasesOutgoing {
		t.Run(tCase.name, func(t *testing.T) {
			data, err := proto.Marshal(tCase.data)
			if err != nil {
				t.Errorf("Can't marshal data: %v", err)
			}

			tVchanPriv.recv <- data

			select {
			case receivedData := <-vch.GetReceivingChannel():
				outgoingData := &pb.SMOutgoingMessages{}
				if err := proto.Unmarshal(receivedData, outgoingData); err != nil {
					t.Errorf("Can't unmarshal data: %v", err)
				}

				if !proto.Equal(tCase.data, outgoingData) {
					t.Errorf("Expected data: %s, received data: %s", tCase.data, outgoingData)
				}

			case <-tVchanPub.recv:
				t.Errorf("Unexpected data")

			case <-time.After(1 * time.Second):
				t.Errorf("Timeout")
			}
		})
	}
}

func TestPublicReadWriteVchan(t *testing.T) {
	tVchanPriv := &testVChan{
		send: make(chan []byte, 1),
		recv: make(chan []byte, 1),
	}

	tVchanPub := &testVChan{
		send: make(chan []byte, 1),
		recv: make(chan []byte, 1),
	}

	vch, err := vchanmanager.New(nil, nil, tVchanPub, tVchanPriv)
	if err != nil {
		t.Errorf("Can't create a new vchannel manager: %v", err)
	}
	defer vch.Close()

	tCases := []struct {
		name string
		data *pb.SMIncomingMessages
	}{
		{
			name: "Test 1",
			data: &pb.SMIncomingMessages{
				SMIncomingMessage: &pb.SMIncomingMessages_ClockSync{
					ClockSync: &pb.ClockSync{
						CurrentTime: &timestamppb.Timestamp{
							Seconds: 1,
							Nanos:   1,
						},
					},
				},
			},
		},
		{
			name: "Test 2",
			data: &pb.SMIncomingMessages{
				SMIncomingMessage: &pb.SMIncomingMessages_ClockSync{
					ClockSync: &pb.ClockSync{
						CurrentTime: &timestamppb.Timestamp{
							Seconds: 2,
							Nanos:   2,
						},
					},
				},
			},
		},
	}

	for _, tCase := range tCases {
		t.Run(tCase.name, func(t *testing.T) {
			data, err := proto.Marshal(tCase.data)
			if err != nil {
				t.Errorf("Can't marshal data: %v", err)
			}

			vch.GetSendingChannel() <- data

			select {
			case receivedData := <-tVchanPub.send:
				incomingData := &pb.SMIncomingMessages{}
				if err := proto.Unmarshal(receivedData, incomingData); err != nil {
					t.Errorf("Can't unmarshal data: %v", err)
				}

				if !proto.Equal(tCase.data, incomingData) {
					t.Errorf("Expected data: %s, received data: %s", tCase.data, incomingData)
				}

			case <-tVchanPriv.send:
				t.Errorf("Unexpected data")

			case <-time.After(1 * time.Second):
				t.Errorf("Timeout")
			}
		})
	}

	tCasesOutgoing := []struct {
		name string
		data *pb.SMOutgoingMessages
	}{
		{
			name: "Test 1",
			data: &pb.SMOutgoingMessages{
				SMOutgoingMessage: &pb.SMOutgoingMessages_NodeMonitoring{
					NodeMonitoring: &pb.NodeMonitoring{
						Timestamp: &timestamppb.Timestamp{
							Seconds: 1,
							Nanos:   1,
						},
						MonitoringData: &pb.MonitoringData{
							Ram: 10,
							Cpu: 20,
						},
					},
				},
			},
		},
		{
			name: "Test 2",
			data: &pb.SMOutgoingMessages{
				SMOutgoingMessage: &pb.SMOutgoingMessages_NodeMonitoring{
					NodeMonitoring: &pb.NodeMonitoring{
						Timestamp: &timestamppb.Timestamp{
							Seconds: 2,
							Nanos:   2,
						},
						MonitoringData: &pb.MonitoringData{
							Ram: 20,
							Cpu: 40,
						},
					},
				},
			},
		},
	}

	for _, tCase := range tCasesOutgoing {
		t.Run(tCase.name, func(t *testing.T) {
			data, err := proto.Marshal(tCase.data)
			if err != nil {
				t.Errorf("Can't marshal data: %v", err)
			}

			tVchanPub.recv <- data

			select {
			case receivedData := <-vch.GetReceivingChannel():
				outgoingData := &pb.SMOutgoingMessages{}
				if err := proto.Unmarshal(receivedData, outgoingData); err != nil {
					t.Errorf("Can't unmarshal data: %v", err)
				}

				if !proto.Equal(tCase.data, outgoingData) {
					t.Errorf("Expected data: %s, received data: %s", tCase.data, outgoingData)
				}

			case <-tVchanPriv.recv:
				t.Errorf("Unexpected data")

			case <-time.After(1 * time.Second):
				t.Errorf("Timeout")
			}
		})
	}
}

func TestDownload(t *testing.T) {
	tVchanPriv := &testVChan{
		send: make(chan []byte, 1),
		recv: make(chan []byte, 1),
	}

	tVchanPub := &testVChan{
		send: make(chan []byte, 1),
		recv: make(chan []byte, 1),
	}

	tmpDir, err := os.MkdirTemp("", "vchan_")
	if err != nil {
		t.Fatalf("Can't create a temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fileName := path.Join(tmpDir, "package.txt")

	if err := generateFile(fileName, 1*Kilobyte); err != nil {
		t.Fatalf("Can't generate file: %v", err)
	}

	vch, err := vchanmanager.New(&testDownloader{
		downloadedFile: fileName,
	}, &testUnpacker{
		filePath: tmpDir,
	}, tVchanPub, tVchanPriv)
	if err != nil {
		t.Errorf("Can't create a new communication manager: %v", err)
	}
	defer vch.Close()

	file, err := os.Open(fileName)
	if err != nil {
		t.Fatalf("Can't open file: %v", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatalf("Can't calculate hash: %v", err)
	}

	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("Can't seek file: %v", err)
	}

	data := make([]byte, 1*Kilobyte)

	if data, err = io.ReadAll(file); err != nil {
		t.Fatalf("Can't read file: %v", err)
	}

	relPath, err := filepath.Rel(tmpDir, fileName)
	if err != nil {
		t.Fatalf("Can't get relative path: %v", err)
	}

	fileInfo, err := file.Stat()
	if err != nil {
		t.Fatalf("Can't get file info: %v", err)
	}

	imageContentRequest := &pb.SMOutgoingMessages{
		SMOutgoingMessage: &pb.SMOutgoingMessages_ImageContentRequest{
			ImageContentRequest: &pb.ImageContentRequest{
				RequestId:   1,
				ContentType: "service",
			},
		},
	}

	imageContentRequestRaw, err := proto.Marshal(imageContentRequest)
	if err != nil {
		t.Fatalf("Can't marshal data: %v", err)
	}

	tVchanPriv.recv <- imageContentRequestRaw

	tCases := []struct {
		name string
		data *pb.SMIncomingMessages
	}{
		{
			name: "Test 1",
			data: &pb.SMIncomingMessages{
				SMIncomingMessage: &pb.SMIncomingMessages_ImageContentInfo{
					ImageContentInfo: &pb.ImageContentInfo{
						RequestId: 1,
						ImageFiles: []*pb.ImageFile{
							{
								RelativePath: relPath,
								Sha256:       hash.Sum(nil),
								Size:         uint64(fileInfo.Size()),
							},
						},
					},
				},
			},
		},
		{
			name: "Test 2",
			data: &pb.SMIncomingMessages{
				SMIncomingMessage: &pb.SMIncomingMessages_ImageContent{
					ImageContent: &pb.ImageContent{
						RequestId:    1,
						PartsCount:   1,
						RelativePath: relPath,
						Part:         1,
						Data:         data[:],
					},
				},
			},
		},
	}

	for _, tCase := range tCases {
		t.Run(tCase.name, func(t *testing.T) {
			select {
			case recievedData := <-tVchanPriv.send:
				data, err := proto.Marshal(tCase.data)
				if err != nil {
					t.Errorf("Can't marshal data: %v", err)
				}

				if !bytes.Equal(recievedData, data) {
					t.Error("Unexpected received data")
				}

			case <-tVchanPub.send:
				t.Errorf("Unexpected data")

			case <-time.After(6 * time.Second):
				t.Errorf("Timeout")
			}
		})
	}
}

/***********************************************************************************************************************
 * Interfaces
 **********************************************************************************************************************/

func (v *testVChan) Connect(ctx context.Context) error {
	return nil
}

func (v *testVChan) ReadMessage() ([]byte, error) {
	return <-v.recv, nil
}

func (v *testVChan) WriteMessage(data []byte) error {
	v.send <- data

	return nil
}

func (v *testVChan) Disconnect() error {
	return nil
}

func (td *testDownloader) Download(
	ctx context.Context, url string,
) (fileName string, err error) {
	return td.downloadedFile, nil
}

func (tu *testUnpacker) Unpack(archivePath string, contentType string) (string, error) {
	return tu.filePath, nil
}

/***********************************************************************************************************************
 * Private
 **********************************************************************************************************************/

func generateFile(fileName string, size uint64) (err error) {
	if output, err := exec.Command("dd", "if=/dev/urandom", "of="+fileName, "bs=1",
		"count="+strconv.FormatUint(size, 10)).CombinedOutput(); err != nil {
		return aoserrors.Errorf("%v (%s)", err, (string(output)))
	}

	return nil
}
