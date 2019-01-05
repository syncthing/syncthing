//+build !noasm

/*
 * Minio Cloud Storage, (C) 2016 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sha256

func blockArmGo(dig *digest, p []byte)  {}
func blockAvx2Go(dig *digest, p []byte) {}
func blockAvxGo(dig *digest, p []byte)  {}
func blockSsseGo(dig *digest, p []byte) {}
func blockShaGo(dig *digest, p []byte)  {}
