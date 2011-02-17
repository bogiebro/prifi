/* libdissent/network.hpp
   Network layer (w/ signing and logging) for dissent protocol.

   Author: Shu-Chun Weng <scweng _AT_ cs .DOT. yale *DOT* edu>
 */
/* ====================================================================
 * Dissent: Accountable Group Anonymity
 * Copyright (c) 2010 Yale University.
 *
 * This library is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public
 * License as published by the Free Software Foundation; either
 * version 2.1 of the License, or (at your option) any later version.
 *
 * This library is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 * Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public
 * License along with this library; if not, write to
 *
 *   Free Software Foundation, Inc.,
 *   51 Franklin Street, Fifth Floor,
 *   Boston, MA  02110-1301  USA
 */

#ifndef _DISSENT_LIBDISSENT_NETWORK_H_
#define _DISSENT_LIBDISSENT_NETWORK_H_ 1

#include <QtGlobal>
#include <QByteArray>
#include <QList>
#include <QMap>
#include <QObject>
#include <QTcpServer>
#include <QTcpSocket>
#include <QQueue>

namespace Dissent{
class Node;

class Network : public QObject{
  Q_OBJECT
  public:
    Network(Node* node);

    void SetNonce(qint32 word){ _nonce = word; }

    int Send(int node_id, const QByteArray& data);
    int Broadcast(const QByteArray& data);

    int Read(int node_id, QByteArray* data);

    struct LogEntry{
        enum{ SEND, RECV }dir;
        int node_id;
        QByteArray data;
        QByteArray signature;

        bool valid;
    };

    void ClearLog(){ _log.clear(); }
    const QList<LogEntry>& GetLog() const{ return _log; }

  signals:
    void readyRead(int node_id);
    void inputError(int node_id);

  protected slots:
    void ClientHaveReadyRead(int node_id);

    void StartIncomingNetwork();
    void StopIncomingNetwork();

  private:
    Node* _node;
    QList<LogEntry> _log;

    QTcpServer _server;
    QMap<int, QTcpSocket> _clients;
    QMap<QTcpSocket*, int> _clientNodeId;

    struct Buffer{
        // data_left < 0: nothing buffered
        // data_left == 0: buffering signature
        // data_left > 0: next data_left bytes are data body
        int data_left;
        int sig_left;

        QByteArray data;
        QByteArray sig;
    };

    // index of message in _log
    QQueue<int> _readyQueue;
    bool _inReceivingPhase;

    qint32 _nonce;
};
}
#endif  // _DISSENT_LIBDISSENT_NETWORK_H_
// -*- vim:sw=4:expandtab:cindent:
